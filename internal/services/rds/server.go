package rds

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
)

const (
	xmlContentType = "text/xml"
	rdsXMLNS       = "http://rds.amazonaws.com/doc/2014-10-31/"
)

type DBInstance struct {
	DBInstanceIdentifier string
	DBInstanceClass      string
	Engine               string
	DBInstanceStatus     string
	Endpoint             Endpoint
	MasterUsername       string
	DBName               string
}

type Endpoint struct {
	Address string
	Port    int
}

type DBSnapshot struct {
	DBSnapshotIdentifier string
	DBInstanceIdentifier string
	Status               string
}

type DBParameterGroup struct {
	DBParameterGroupName   string
	DBParameterGroupFamily string
	Description            string
}

type server struct {
	mu            sync.RWMutex
	dbInstances   map[string]DBInstance
	dbSnapshots   map[string]DBSnapshot
	dbParamGroups map[string]DBParameterGroup
}

func newServer() *server {
	return &server{
		dbInstances:   make(map[string]DBInstance),
		dbSnapshots:   make(map[string]DBSnapshot),
		dbParamGroups: make(map[string]DBParameterGroup),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "InvalidAction", "Only POST is supported")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, "InvalidParameterValue", "Could not parse form body")
		return
	}

	switch r.FormValue("Action") {
	case "CreateDBInstance":
		s.createDBInstance(w, r)
	case "DeleteDBInstance":
		s.deleteDBInstance(w, r)
	case "DescribeDBInstances":
		s.describeDBInstances(w)
	case "StopDBInstance":
		s.stopDBInstance(w, r)
	case "StartDBInstance":
		s.startDBInstance(w, r)
	case "CreateDBSnapshot":
		s.createDBSnapshot(w, r)
	case "DescribeDBSnapshots":
		s.describeDBSnapshots(w)
	case "CreateDBParameterGroup":
		s.createDBParameterGroup(w, r)
	case "DescribeDBParameterGroups":
		s.describeDBParameterGroups(w)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) createDBInstance(w http.ResponseWriter, r *http.Request) {
	identifier := r.FormValue("DBInstanceIdentifier")
	class := r.FormValue("DBInstanceClass")
	engine := r.FormValue("Engine")
	masterUsername := r.FormValue("MasterUsername")
	dbName := r.FormValue("DBName")
	if identifier == "" || class == "" || engine == "" || masterUsername == "" {
		writeError(w, "MissingParameter", "DBInstanceIdentifier, DBInstanceClass, Engine, and MasterUsername are required")
		return
	}

	port := 3306
	if engine == "postgres" {
		port = 5432
	}
	instance := DBInstance{identifier, class, engine, "available", Endpoint{"localhost", port}, masterUsername, dbName}

	s.mu.Lock()
	s.dbInstances[identifier] = instance
	s.mu.Unlock()

	writeXML(w, createDBInstanceResponse{XMLNS: rdsXMLNS, Result: dbInstanceResult{DBInstance: toDBInstanceMember(instance)}})
}

func (s *server) deleteDBInstance(w http.ResponseWriter, r *http.Request) {
	identifier := r.FormValue("DBInstanceIdentifier")
	if identifier == "" {
		writeError(w, "MissingParameter", "DBInstanceIdentifier is required")
		return
	}
	s.mu.Lock()
	delete(s.dbInstances, identifier)
	s.mu.Unlock()
	writeXML(w, deleteDBInstanceResponse{XMLNS: rdsXMLNS})
}

func (s *server) describeDBInstances(w http.ResponseWriter) {
	s.mu.RLock()
	instances := make([]DBInstance, 0, len(s.dbInstances))
	for _, i := range s.dbInstances {
		instances = append(instances, i)
	}
	s.mu.RUnlock()
	sort.Slice(instances, func(i, j int) bool { return instances[i].DBInstanceIdentifier < instances[j].DBInstanceIdentifier })
	members := make([]dbInstanceMember, 0, len(instances))
	for _, i := range instances {
		members = append(members, toDBInstanceMember(i))
	}
	writeXML(w, describeDBInstancesResponse{XMLNS: rdsXMLNS, Result: describeDBInstancesResult{DBInstances: dbInstanceList{Members: members}}})
}

func (s *server) stopDBInstance(w http.ResponseWriter, r *http.Request) {
	s.setInstanceStatus(w, r, "stopped", "StopDBInstanceResponse")
}
func (s *server) startDBInstance(w http.ResponseWriter, r *http.Request) {
	s.setInstanceStatus(w, r, "available", "StartDBInstanceResponse")
}

func (s *server) setInstanceStatus(w http.ResponseWriter, r *http.Request, status, action string) {
	identifier := r.FormValue("DBInstanceIdentifier")
	if identifier == "" {
		writeError(w, "MissingParameter", "DBInstanceIdentifier is required")
		return
	}
	s.mu.Lock()
	instance, ok := s.dbInstances[identifier]
	if ok {
		instance.DBInstanceStatus = status
		s.dbInstances[identifier] = instance
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, "DBInstanceNotFound", "DB instance does not exist")
		return
	}
	if action == "StopDBInstanceResponse" {
		writeXML(w, stopDBInstanceResponse{XMLNS: rdsXMLNS, Result: dbInstanceResult{DBInstance: toDBInstanceMember(instance)}})
		return
	}
	writeXML(w, startDBInstanceResponse{XMLNS: rdsXMLNS, Result: dbInstanceResult{DBInstance: toDBInstanceMember(instance)}})
}

func (s *server) createDBSnapshot(w http.ResponseWriter, r *http.Request) {
	instanceID := r.FormValue("DBInstanceIdentifier")
	snapshotID := r.FormValue("DBSnapshotIdentifier")
	if instanceID == "" || snapshotID == "" {
		writeError(w, "MissingParameter", "DBInstanceIdentifier and DBSnapshotIdentifier are required")
		return
	}
	snapshot := DBSnapshot{DBSnapshotIdentifier: snapshotID, DBInstanceIdentifier: instanceID, Status: "available"}
	s.mu.Lock()
	s.dbSnapshots[snapshotID] = snapshot
	s.mu.Unlock()
	writeXML(w, createDBSnapshotResponse{XMLNS: rdsXMLNS, Result: dbSnapshotResult{DBSnapshot: dbSnapshotMember(snapshot)}})
}

func (s *server) describeDBSnapshots(w http.ResponseWriter) {
	s.mu.RLock()
	snapshots := make([]DBSnapshot, 0, len(s.dbSnapshots))
	for _, v := range s.dbSnapshots {
		snapshots = append(snapshots, v)
	}
	s.mu.RUnlock()
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].DBSnapshotIdentifier < snapshots[j].DBSnapshotIdentifier })
	members := make([]dbSnapshotMember, 0, len(snapshots))
	for _, v := range snapshots {
		members = append(members, dbSnapshotMember(v))
	}
	writeXML(w, describeDBSnapshotsResponse{XMLNS: rdsXMLNS, Result: describeDBSnapshotsResult{DBSnapshots: dbSnapshotList{Members: members}}})
}

func (s *server) createDBParameterGroup(w http.ResponseWriter, r *http.Request) {
	name, family, desc := r.FormValue("DBParameterGroupName"), r.FormValue("DBParameterGroupFamily"), r.FormValue("Description")
	if name == "" || family == "" || desc == "" {
		writeError(w, "MissingParameter", "DBParameterGroupName, DBParameterGroupFamily, and Description are required")
		return
	}
	pg := DBParameterGroup{name, family, desc}
	s.mu.Lock()
	s.dbParamGroups[name] = pg
	s.mu.Unlock()
	writeXML(w, createDBParameterGroupResponse{XMLNS: rdsXMLNS, Result: dbParameterGroupResult{DBParameterGroup: dbParameterGroupMember(pg)}})
}

func (s *server) describeDBParameterGroups(w http.ResponseWriter) {
	s.mu.RLock()
	pgs := make([]DBParameterGroup, 0, len(s.dbParamGroups))
	for _, v := range s.dbParamGroups {
		pgs = append(pgs, v)
	}
	s.mu.RUnlock()
	sort.Slice(pgs, func(i, j int) bool { return pgs[i].DBParameterGroupName < pgs[j].DBParameterGroupName })
	members := make([]dbParameterGroupMember, 0, len(pgs))
	for _, v := range pgs {
		members = append(members, dbParameterGroupMember(v))
	}
	writeXML(w, describeDBParameterGroupsResponse{XMLNS: rdsXMLNS, Result: describeDBParameterGroupsResult{DBParameterGroups: dbParameterGroupList{Members: members}}})
}

func writeXML(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, errorResponse{XMLNS: rdsXMLNS, Error: errorBody{Type: "Sender", Code: code, Message: message}, RequestID: "req-error"})
}

type errorResponse struct {
	XMLName   xml.Name  `xml:"ErrorResponse"`
	XMLNS     string    `xml:"xmlns,attr"`
	Error     errorBody `xml:"Error"`
	RequestID string    `xml:"RequestId"`
}
type errorBody struct{ Type, Code, Message string }
type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

type dbInstanceMember struct {
	DBInstanceIdentifier, DBInstanceClass, Engine, DBInstanceStatus, MasterUsername, DBName string
	Endpoint                                                                                endpointMember
}
type endpointMember struct {
	Address string
	Port    string
}

func toDBInstanceMember(in DBInstance) dbInstanceMember {
	return dbInstanceMember{in.DBInstanceIdentifier, in.DBInstanceClass, in.Engine, in.DBInstanceStatus, in.MasterUsername, in.DBName, endpointMember{in.Endpoint.Address, strconv.Itoa(in.Endpoint.Port)}}
}

type dbInstanceList struct {
	Members []dbInstanceMember `xml:"member"`
}
type dbInstanceResult struct {
	DBInstance dbInstanceMember `xml:"DBInstance"`
}

type createDBInstanceResponse struct {
	XMLName  xml.Name         `xml:"CreateDBInstanceResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	Result   dbInstanceResult `xml:"CreateDBInstanceResult"`
	Metadata responseMetadata `xml:"ResponseMetadata"`
}
type deleteDBInstanceResponse struct {
	XMLName  xml.Name         `xml:"DeleteDBInstanceResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	Metadata responseMetadata `xml:"ResponseMetadata"`
}
type stopDBInstanceResponse struct {
	XMLName  xml.Name         `xml:"StopDBInstanceResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	Result   dbInstanceResult `xml:"StopDBInstanceResult"`
	Metadata responseMetadata `xml:"ResponseMetadata"`
}
type startDBInstanceResponse struct {
	XMLName  xml.Name         `xml:"StartDBInstanceResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	Result   dbInstanceResult `xml:"StartDBInstanceResult"`
	Metadata responseMetadata `xml:"ResponseMetadata"`
}
type describeDBInstancesResult struct {
	DBInstances dbInstanceList `xml:"DBInstances"`
}
type describeDBInstancesResponse struct {
	XMLName  xml.Name                  `xml:"DescribeDBInstancesResponse"`
	XMLNS    string                    `xml:"xmlns,attr"`
	Result   describeDBInstancesResult `xml:"DescribeDBInstancesResult"`
	Metadata responseMetadata          `xml:"ResponseMetadata"`
}

type dbSnapshotMember DBSnapshot
type dbSnapshotList struct {
	Members []dbSnapshotMember `xml:"member"`
}
type dbSnapshotResult struct {
	DBSnapshot dbSnapshotMember `xml:"DBSnapshot"`
}
type createDBSnapshotResponse struct {
	XMLName  xml.Name         `xml:"CreateDBSnapshotResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	Result   dbSnapshotResult `xml:"CreateDBSnapshotResult"`
	Metadata responseMetadata `xml:"ResponseMetadata"`
}
type describeDBSnapshotsResult struct {
	DBSnapshots dbSnapshotList `xml:"DBSnapshots"`
}
type describeDBSnapshotsResponse struct {
	XMLName  xml.Name                  `xml:"DescribeDBSnapshotsResponse"`
	XMLNS    string                    `xml:"xmlns,attr"`
	Result   describeDBSnapshotsResult `xml:"DescribeDBSnapshotsResult"`
	Metadata responseMetadata          `xml:"ResponseMetadata"`
}

type dbParameterGroupMember DBParameterGroup
type dbParameterGroupList struct {
	Members []dbParameterGroupMember `xml:"member"`
}
type dbParameterGroupResult struct {
	DBParameterGroup dbParameterGroupMember `xml:"DBParameterGroup"`
}
type createDBParameterGroupResponse struct {
	XMLName  xml.Name               `xml:"CreateDBParameterGroupResponse"`
	XMLNS    string                 `xml:"xmlns,attr"`
	Result   dbParameterGroupResult `xml:"CreateDBParameterGroupResult"`
	Metadata responseMetadata       `xml:"ResponseMetadata"`
}
type describeDBParameterGroupsResult struct {
	DBParameterGroups dbParameterGroupList `xml:"DBParameterGroups"`
}
type describeDBParameterGroupsResponse struct {
	XMLName  xml.Name                        `xml:"DescribeDBParameterGroupsResponse"`
	XMLNS    string                          `xml:"xmlns,attr"`
	Result   describeDBParameterGroupsResult `xml:"DescribeDBParameterGroupsResult"`
	Metadata responseMetadata                `xml:"ResponseMetadata"`
}
