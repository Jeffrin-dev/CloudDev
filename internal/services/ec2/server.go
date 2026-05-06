package ec2

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const xmlContentType = "text/xml"
const apiNamespace = "http://ec2.amazonaws.com/doc/2016-11-15/"

type Instance struct{ InstanceId, ImageId, InstanceType, State, PrivateIpAddress, PublicIpAddress, KeyName string }
type Image struct{ ImageId, Name, State, Description string }
type KeyPair struct{ KeyName, KeyFingerprint, KeyMaterial string }
type SecurityGroup struct{ GroupId, GroupName, Description string }

type server struct {
	mu             sync.Mutex
	instances      map[string]Instance
	images         map[string]Image
	keyPairs       map[string]KeyPair
	securityGroups map[string]SecurityGroup
	nextIP         int
}

func newServer() *server {
	return &server{instances: map[string]Instance{}, images: map[string]Image{}, keyPairs: map[string]KeyPair{}, securityGroups: map[string]SecurityGroup{}, nextIP: 1}
}

func Start(port int) error {
	if port == 0 {
		port = 4600
	}
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
	case "RunInstances":
		s.runInstances(w, r)
	case "DescribeInstances":
		s.describeInstances(w)
	case "TerminateInstances":
		s.setInstanceState(w, r, "terminated", "TerminateInstancesResponse")
	case "StopInstances":
		s.setInstanceState(w, r, "stopped", "StopInstancesResponse")
	case "StartInstances":
		s.setInstanceState(w, r, "running", "StartInstancesResponse")
	case "DescribeImages":
		s.describeImages(w)
	case "DescribeKeyPairs":
		s.describeKeyPairs(w)
	case "CreateKeyPair":
		s.createKeyPair(w, r)
	case "DeleteKeyPair":
		s.deleteKeyPair(w, r)
	case "DescribeSecurityGroups":
		s.describeSecurityGroups(w)
	case "CreateSecurityGroup":
		s.createSecurityGroup(w, r)
	case "DeleteSecurityGroup":
		s.deleteSecurityGroup(w, r)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) runInstances(w http.ResponseWriter, r *http.Request) {
	minCount, _ := strconv.Atoi(r.FormValue("MinCount"))
	if minCount <= 0 {
		minCount = 1
	}
	maxCount, _ := strconv.Atoi(r.FormValue("MaxCount"))
	if maxCount < minCount {
		maxCount = minCount
	}
	count := minCount
	imageID := r.FormValue("ImageId")
	if imageID == "" {
		imageID = "ami-00000001"
	}
	itype := r.FormValue("InstanceType")
	if itype == "" {
		itype = "t2.micro"
	}
	keyName := r.FormValue("KeyName")
	s.mu.Lock()
	defer s.mu.Unlock()
	members := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := "i-" + randomHex(4)
		ip := s.nextIP
		s.nextIP++
		inst := Instance{id, imageID, itype, "running", fmt.Sprintf("10.0.0.%d", ip), fmt.Sprintf("54.0.0.%d", ip), keyName}
		s.instances[id] = inst
		members = append(members, instanceXML(inst))
	}
	writeXML(w, fmt.Sprintf("<RunInstancesResponse xmlns=\"%s\"><instancesSet>%s</instancesSet></RunInstancesResponse>", apiNamespace, strings.Join(members, "")))
}

func (s *server) describeInstances(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.instances))
	for id := range s.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	members := ""
	for _, id := range ids {
		members += instanceXML(s.instances[id])
	}
	writeXML(w, fmt.Sprintf("<DescribeInstancesResponse xmlns=\"%s\"><reservationSet><item><instancesSet>%s</instancesSet></item></reservationSet></DescribeInstancesResponse>", apiNamespace, members))
}

func (s *server) setInstanceState(w http.ResponseWriter, r *http.Request, state string, root string) {
	ids := r.Form["InstanceId"]
	if len(ids) == 0 {
		if v := r.FormValue("InstanceId"); v != "" {
			ids = []string{v}
		}
	}
	s.mu.Lock()
	for _, id := range ids {
		if inst, ok := s.instances[id]; ok {
			inst.State = state
			s.instances[id] = inst
		}
	}
	s.mu.Unlock()
	writeXML(w, fmt.Sprintf("<%s xmlns=\"%s\"></%s>", root, apiNamespace, root))
}

func (s *server) describeImages(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := "<item><imageId>ami-00000001</imageId><name>amazon-linux-2</name><imageState>available</imageState><description>Default image</description></item>"
	ids := make([]string, 0, len(s.images))
	for id := range s.images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		img := s.images[id]
		members += fmt.Sprintf("<item><imageId>%s</imageId><name>%s</name><imageState>%s</imageState><description>%s</description></item>", img.ImageId, img.Name, img.State, img.Description)
	}
	writeXML(w, fmt.Sprintf("<DescribeImagesResponse xmlns=\"%s\"><imagesSet>%s</imagesSet></DescribeImagesResponse>", apiNamespace, members))
}
func (s *server) describeKeyPairs(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.keyPairs))
	for n := range s.keyPairs {
		names = append(names, n)
	}
	sort.Strings(names)
	members := ""
	for _, n := range names {
		k := s.keyPairs[n]
		members += fmt.Sprintf("<item><keyName>%s</keyName><keyFingerprint>%s</keyFingerprint></item>", k.KeyName, k.KeyFingerprint)
	}
	writeXML(w, fmt.Sprintf("<DescribeKeyPairsResponse xmlns=\"%s\"><keySet>%s</keySet></DescribeKeyPairsResponse>", apiNamespace, members))
}
func (s *server) createKeyPair(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("KeyName")
	if name == "" {
		name = "key-" + randomHex(2)
	}
	kp := KeyPair{name, "aa:bb:cc:dd", "-----BEGIN PRIVATE KEY-----\nMOCK\n-----END PRIVATE KEY-----"}
	s.mu.Lock()
	s.keyPairs[name] = kp
	s.mu.Unlock()
	writeXML(w, fmt.Sprintf("<CreateKeyPairResponse xmlns=\"%s\"><keyName>%s</keyName><keyFingerprint>%s</keyFingerprint><keyMaterial>%s</keyMaterial></CreateKeyPairResponse>", apiNamespace, kp.KeyName, kp.KeyFingerprint, kp.KeyMaterial))
}
func (s *server) deleteKeyPair(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	delete(s.keyPairs, r.FormValue("KeyName"))
	s.mu.Unlock()
	writeXML(w, fmt.Sprintf("<DeleteKeyPairResponse xmlns=\"%s\"></DeleteKeyPairResponse>", apiNamespace))
}
func (s *server) describeSecurityGroups(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.securityGroups))
	for id := range s.securityGroups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	members := ""
	for _, id := range ids {
		g := s.securityGroups[id]
		members += fmt.Sprintf("<item><groupId>%s</groupId><groupName>%s</groupName><groupDescription>%s</groupDescription></item>", g.GroupId, g.GroupName, g.Description)
	}
	writeXML(w, fmt.Sprintf("<DescribeSecurityGroupsResponse xmlns=\"%s\"><securityGroupInfo>%s</securityGroupInfo></DescribeSecurityGroupsResponse>", apiNamespace, members))
}
func (s *server) createSecurityGroup(w http.ResponseWriter, r *http.Request) {
	g := SecurityGroup{"sg-" + randomHex(4), r.FormValue("GroupName"), r.FormValue("Description")}
	s.mu.Lock()
	s.securityGroups[g.GroupId] = g
	s.mu.Unlock()
	writeXML(w, fmt.Sprintf("<CreateSecurityGroupResponse xmlns=\"%s\"><groupId>%s</groupId></CreateSecurityGroupResponse>", apiNamespace, g.GroupId))
}
func (s *server) deleteSecurityGroup(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	delete(s.securityGroups, r.FormValue("GroupId"))
	s.mu.Unlock()
	writeXML(w, fmt.Sprintf("<DeleteSecurityGroupResponse xmlns=\"%s\"></DeleteSecurityGroupResponse>", apiNamespace))
}

func instanceXML(i Instance) string {
	return fmt.Sprintf("<item><instanceId>%s</instanceId><imageId>%s</imageId><instanceType>%s</instanceType><instanceState><name>%s</name></instanceState><privateIpAddress>%s</privateIpAddress><ipAddress>%s</ipAddress><keyName>%s</keyName></item>", i.InstanceId, i.ImageId, i.InstanceType, i.State, i.PrivateIpAddress, i.PublicIpAddress, i.KeyName)
}
func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}
func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<Response><Errors><Error><Code>%s</Code><Message>%s</Message></Error></Errors></Response>", code, message))
}
func randomHex(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }
