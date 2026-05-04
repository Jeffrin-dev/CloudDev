package ecs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"

const targetPrefix = "AmazonEC2ContainerServiceV20141113."

type Cluster struct {
	ClusterName         string
	ClusterArn          string
	Status              string
	ActiveServicesCount int
	RunningTasksCount   int
}

type TaskDefinition struct {
	Family               string
	Revision             int
	TaskDefinitionArn    string
	Status               string
	ContainerDefinitions []map[string]interface{}
}

type Task struct {
	TaskArn           string
	ClusterArn        string
	TaskDefinitionArn string
	LastStatus        string
	DesiredStatus     string
}

type server struct {
	mu            sync.RWMutex
	clusters      map[string]*Cluster
	taskDefs      map[string][]*TaskDefinition
	taskDefsByARN map[string]*TaskDefinition
	tasks         map[string]*Task
	taskCounter   int
}

func newServer() *server {
	return &server{clusters: make(map[string]*Cluster), taskDefs: make(map[string][]*TaskDefinition), taskDefsByARN: make(map[string]*TaskDefinition), tasks: make(map[string]*Task)}
}

func Start(port int) error { return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer()) }

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "Only POST is supported")
		return
	}
	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Missing X-Amz-Target")
		return
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "SerializationException", "Invalid JSON body")
		return
	}
	switch target {
	case targetPrefix + "CreateCluster":
		s.handleCreateCluster(w, payload)
	case targetPrefix + "DeleteCluster":
		s.handleDeleteCluster(w, payload)
	case targetPrefix + "ListClusters":
		s.handleListClusters(w)
	case targetPrefix + "DescribeClusters":
		s.handleDescribeClusters(w, payload)
	case targetPrefix + "RegisterTaskDefinition":
		s.handleRegisterTaskDefinition(w, payload)
	case targetPrefix + "ListTaskDefinitions":
		s.handleListTaskDefinitions(w)
	case targetPrefix + "DescribeTaskDefinition":
		s.handleDescribeTaskDefinition(w, payload)
	case targetPrefix + "RunTask":
		s.handleRunTask(w, payload)
	case targetPrefix + "StopTask":
		s.handleStopTask(w, payload)
	case targetPrefix + "ListTasks":
		s.handleListTasks(w, payload)
	case targetPrefix + "DescribeTasks":
		s.handleDescribeTasks(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateCluster(w http.ResponseWriter, payload map[string]interface{}) {
	name, _ := stringField(payload, "clusterName")
	if name == "" {
		name, _ = stringField(payload, "cluster")
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "clusterName is required")
		return
	}
	cluster := &Cluster{ClusterName: name, ClusterArn: fmt.Sprintf("arn:aws:ecs:us-east-1:000000000000:cluster/%s", name), Status: "ACTIVE"}
	s.mu.Lock()
	s.clusters[name] = cluster
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"cluster": cluster})
}
func (s *server) handleDeleteCluster(w http.ResponseWriter, payload map[string]interface{}) {
	v, _ := stringField(payload, "cluster")
	name := clusterNameFromRef(v)
	if name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "cluster is required")
		return
	}
	s.mu.Lock()
	cluster, ok := s.clusters[name]
	if ok {
		delete(s.clusters, name)
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusBadRequest, "ClusterNotFoundException", "Cluster not found")
		return
	}
	cluster.Status = "INACTIVE"
	writeJSON(w, http.StatusOK, map[string]interface{}{"cluster": cluster})
}
func (s *server) handleListClusters(w http.ResponseWriter) {
	s.mu.RLock()
	arns := make([]string, 0, len(s.clusters))
	for _, c := range s.clusters {
		arns = append(arns, c.ClusterArn)
	}
	s.mu.RUnlock()
	sort.Strings(arns)
	writeJSON(w, http.StatusOK, map[string]interface{}{"clusterArns": arns})
}
func (s *server) handleDescribeClusters(w http.ResponseWriter, payload map[string]interface{}) {
	ids := stringSliceField(payload, "clusters")
	out := make([]*Cluster, 0, len(ids))
	s.mu.RLock()
	for _, id := range ids {
		if c, ok := s.clusters[clusterNameFromRef(id)]; ok {
			out = append(out, c)
		}
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"clusters": out})
}
func (s *server) handleRegisterTaskDefinition(w http.ResponseWriter, payload map[string]interface{}) {
	family, _ := stringField(payload, "family")
	if family == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "family is required")
		return
	}
	containerDefs := toMapSlice(payload["containerDefinitions"])
	s.mu.Lock()
	rev := len(s.taskDefs[family]) + 1
	arn := fmt.Sprintf("arn:aws:ecs:us-east-1:000000000000:task-definition/%s:%d", family, rev)
	td := &TaskDefinition{Family: family, Revision: rev, TaskDefinitionArn: arn, Status: "ACTIVE", ContainerDefinitions: containerDefs}
	s.taskDefs[family] = append(s.taskDefs[family], td)
	s.taskDefsByARN[arn] = td
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"taskDefinition": td})
}
func (s *server) handleListTaskDefinitions(w http.ResponseWriter) {
	s.mu.RLock()
	arns := make([]string, 0, len(s.taskDefsByARN))
	for arn := range s.taskDefsByARN {
		arns = append(arns, arn)
	}
	s.mu.RUnlock()
	sort.Strings(arns)
	writeJSON(w, http.StatusOK, map[string]interface{}{"taskDefinitionArns": arns})
}
func (s *server) handleDescribeTaskDefinition(w http.ResponseWriter, payload map[string]interface{}) {
	ref, _ := stringField(payload, "taskDefinition")
	td := s.resolveTaskDefinition(ref)
	if td == nil {
		writeError(w, http.StatusBadRequest, "ClientException", "Unable to describe task definition")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"taskDefinition": td})
}
func (s *server) handleRunTask(w http.ResponseWriter, payload map[string]interface{}) {
	clusterRef, _ := stringField(payload, "cluster")
	tdRef, _ := stringField(payload, "taskDefinition")
	if clusterRef == "" || tdRef == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "cluster and taskDefinition are required")
		return
	}
	clusterName := clusterNameFromRef(clusterRef)
	s.mu.Lock()
	cluster, ok := s.clusters[clusterName]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "ClusterNotFoundException", "Cluster not found")
		return
	}
	td := s.resolveTaskDefinitionLocked(tdRef)
	if td == nil {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "ClientException", "Unable to run task")
		return
	}
	s.taskCounter++
	taskArn := fmt.Sprintf("arn:aws:ecs:us-east-1:000000000000:task/%012d", s.taskCounter)
	task := &Task{TaskArn: taskArn, ClusterArn: cluster.ClusterArn, TaskDefinitionArn: td.TaskDefinitionArn, LastStatus: "RUNNING", DesiredStatus: "RUNNING"}
	s.tasks[taskArn] = task
	cluster.RunningTasksCount++
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": []*Task{task}})
}
func (s *server) handleStopTask(w http.ResponseWriter, payload map[string]interface{}) {
	taskArn, _ := stringField(payload, "task")
	if taskArn == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "task is required")
		return
	}
	s.mu.Lock()
	task, ok := s.tasks[taskArn]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "TaskNotFoundException", "Task not found")
		return
	}
	task.LastStatus = "STOPPED"
	task.DesiredStatus = "STOPPED"
	if c := s.clusterByARNLocked(task.ClusterArn); c != nil && c.RunningTasksCount > 0 {
		c.RunningTasksCount--
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"task": task})
}
func (s *server) handleListTasks(w http.ResponseWriter, payload map[string]interface{}) {
	clusterRef, _ := stringField(payload, "cluster")
	statusFilter, _ := stringField(payload, "desiredStatus")
	clusterArn := ""
	if clusterRef != "" {
		clusterArn = fmt.Sprintf("arn:aws:ecs:us-east-1:000000000000:cluster/%s", clusterNameFromRef(clusterRef))
	}
	s.mu.RLock()
	arns := []string{}
	for arn, t := range s.tasks {
		if clusterArn != "" && t.ClusterArn != clusterArn {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(t.DesiredStatus, statusFilter) {
			continue
		}
		arns = append(arns, arn)
	}
	s.mu.RUnlock()
	sort.Strings(arns)
	writeJSON(w, http.StatusOK, map[string]interface{}{"taskArns": arns})
}
func (s *server) handleDescribeTasks(w http.ResponseWriter, payload map[string]interface{}) {
	ids := stringSliceField(payload, "tasks")
	out := []*Task{}
	s.mu.RLock()
	for _, id := range ids {
		if t, ok := s.tasks[id]; ok {
			out = append(out, t)
		}
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": out})
}

func (s *server) resolveTaskDefinition(ref string) *TaskDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resolveTaskDefinitionLocked(ref)
}
func (s *server) resolveTaskDefinitionLocked(ref string) *TaskDefinition {
	if ref == "" {
		return nil
	}
	if td, ok := s.taskDefsByARN[ref]; ok {
		return td
	}
	family, rev := parseTaskDefRef(ref)
	defs := s.taskDefs[family]
	if len(defs) == 0 {
		return nil
	}
	if rev == 0 {
		return defs[len(defs)-1]
	}
	for _, td := range defs {
		if td.Revision == rev {
			return td
		}
	}
	return nil
}
func (s *server) clusterByARNLocked(arn string) *Cluster {
	for _, c := range s.clusters {
		if c.ClusterArn == arn {
			return c
		}
	}
	return nil
}

func clusterNameFromRef(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}
func parseTaskDefRef(ref string) (string, int) {
	if strings.Contains(ref, "task-definition/") {
		ref = ref[strings.LastIndex(ref, "/")+1:]
	}
	parts := strings.Split(ref, ":")
	if len(parts) == 1 {
		return parts[0], 0
	}
	var rev int
	fmt.Sscanf(parts[len(parts)-1], "%d", &rev)
	return strings.Join(parts[:len(parts)-1], ":"), rev
}
func toMapSlice(v interface{}) []map[string]interface{} {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}
func stringField(payload map[string]interface{}, key string) (string, bool) {
	v, ok := payload[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
func stringSliceField(payload map[string]interface{}, key string) []string {
	v, ok := payload[key]
	if !ok {
		return nil
	}
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, it := range raw {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{"__type": code, "message": message})
}
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
