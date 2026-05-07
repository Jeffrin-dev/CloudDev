package codepipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"
const pipelineARNFormat = "arn:aws:codepipeline:us-east-1:000000000000:%s"

type Pipeline struct {
	Name    string  `json:"name"`
	RoleArn string  `json:"roleArn"`
	Stages  []Stage `json:"stages"`
}

type Stage struct {
	Name    string   `json:"name"`
	Actions []Action `json:"actions"`
}

type Action struct {
	Name         string            `json:"name"`
	ActionTypeId map[string]string `json:"actionTypeId"`
}

type PipelineExecution struct {
	PipelineExecutionId string `json:"pipelineExecutionId"`
	PipelineName        string `json:"pipelineName"`
	Status              string `json:"status"`
}

type server struct {
	mu         sync.RWMutex
	pipelines  map[string]Pipeline
	executions map[string]map[string]PipelineExecution
}

func newServer() *server {
	return &server{pipelines: map[string]Pipeline{}, executions: map[string]map[string]PipelineExecution{}}
}
func Start(port int) error { return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer()) }

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "Only POST is supported")
		return
	}
	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "Missing X-Amz-Target header")
		return
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "Invalid JSON body")
		return
	}
	switch target {
	case "CodePipeline_20150709.CreatePipeline":
		s.handleCreatePipeline(w, payload)
	case "CodePipeline_20150709.DeletePipeline":
		s.handleDeletePipeline(w, payload)
	case "CodePipeline_20150709.GetPipeline":
		s.handleGetPipeline(w, payload)
	case "CodePipeline_20150709.ListPipelines":
		s.handleListPipelines(w)
	case "CodePipeline_20150709.StartPipelineExecution":
		s.handleStartPipelineExecution(w, payload)
	case "CodePipeline_20150709.GetPipelineExecution":
		s.handleGetPipelineExecution(w, payload)
	case "CodePipeline_20150709.ListPipelineExecutions":
		s.handleListPipelineExecutions(w, payload)
	case "CodePipeline_20150709.StopPipelineExecution":
		s.handleStopPipelineExecution(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreatePipeline(w http.ResponseWriter, payload map[string]interface{}) {
	pipelineMap, ok := payload["pipeline"].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "pipeline is required")
		return
	}
	pipeline := parsePipeline(pipelineMap)
	if pipeline.Name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "pipeline.name is required")
		return
	}
	s.mu.Lock()
	s.pipelines[pipeline.Name] = pipeline
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"pipeline": pipeline, "pipelineArn": pipelineARN(pipeline.Name)})
}
func (s *server) handleDeletePipeline(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "name is required")
		return
	}
	s.mu.Lock()
	delete(s.pipelines, name)
	delete(s.executions, name)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}
func (s *server) handleGetPipeline(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "name is required")
		return
	}
	s.mu.RLock()
	pipeline, exists := s.pipelines[name]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusNotFound, "PipelineNotFoundException", "pipeline not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"pipeline": pipeline})
}
func (s *server) handleListPipelines(w http.ResponseWriter) {
	s.mu.RLock()
	pipelines := make([]map[string]string, 0, len(s.pipelines))
	for _, p := range s.pipelines {
		pipelines = append(pipelines, map[string]string{"name": p.Name})
	}
	s.mu.RUnlock()
	sort.Slice(pipelines, func(i, j int) bool { return pipelines[i]["name"] < pipelines[j]["name"] })
	writeJSON(w, http.StatusOK, map[string]interface{}{"pipelines": pipelines})
}
func (s *server) handleStartPipelineExecution(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "pipelineName")
	if !ok || name == "" {
		name, ok = stringField(payload, "name")
	}
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "pipelineName is required")
		return
	}
	s.mu.Lock()
	if _, exists := s.pipelines[name]; !exists {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "PipelineNotFoundException", "pipeline not found")
		return
	}
	execID := newUUID()
	ex := PipelineExecution{PipelineExecutionId: execID, PipelineName: name, Status: "InProgress"}
	if s.executions[name] == nil {
		s.executions[name] = map[string]PipelineExecution{}
	}
	s.executions[name][execID] = ex
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"pipelineExecutionId": execID})
}
func (s *server) handleGetPipelineExecution(w http.ResponseWriter, payload map[string]interface{}) {
	name, _ := stringField(payload, "pipelineName")
	execID, _ := stringField(payload, "pipelineExecutionId")
	if name == "" || execID == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "pipelineName and pipelineExecutionId are required")
		return
	}
	s.mu.RLock()
	ex, exists := s.executions[name][execID]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusNotFound, "PipelineExecutionNotFoundException", "pipeline execution not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"pipelineExecution": ex})
}
func (s *server) handleListPipelineExecutions(w http.ResponseWriter, payload map[string]interface{}) {
	name, _ := stringField(payload, "pipelineName")
	if name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "pipelineName is required")
		return
	}
	s.mu.RLock()
	m := s.executions[name]
	executions := make([]PipelineExecution, 0, len(m))
	for _, e := range m {
		executions = append(executions, e)
	}
	s.mu.RUnlock()
	sort.Slice(executions, func(i, j int) bool { return executions[i].PipelineExecutionId < executions[j].PipelineExecutionId })
	writeJSON(w, http.StatusOK, map[string]interface{}{"pipelineExecutionSummaries": executions})
}
func (s *server) handleStopPipelineExecution(w http.ResponseWriter, payload map[string]interface{}) {
	name, _ := stringField(payload, "pipelineName")
	execID, _ := stringField(payload, "pipelineExecutionId")
	if name == "" || execID == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "pipelineName and pipelineExecutionId are required")
		return
	}
	s.mu.Lock()
	ex, exists := s.executions[name][execID]
	if exists {
		ex.Status = "Stopped"
		s.executions[name][execID] = ex
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusNotFound, "PipelineExecutionNotFoundException", "pipeline execution not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func parsePipeline(input map[string]interface{}) Pipeline {
	p := Pipeline{Name: asString(input["name"]), RoleArn: asString(input["roleArn"])}
	rawStages, _ := input["stages"].([]interface{})
	for _, rs := range rawStages {
		sm, ok := rs.(map[string]interface{})
		if !ok {
			continue
		}
		st := Stage{Name: asString(sm["name"])}
		rawActions, _ := sm["actions"].([]interface{})
		for _, ra := range rawActions {
			am, ok := ra.(map[string]interface{})
			if !ok {
				continue
			}
			tid := map[string]string{}
			if t, ok := am["actionTypeId"].(map[string]interface{}); ok {
				for k, v := range t {
					tid[k] = asString(v)
				}
			}
			st.Actions = append(st.Actions, Action{Name: asString(am["name"]), ActionTypeId: tid})
		}
		p.Stages = append(p.Stages, st)
	}
	return p
}
func stringField(payload map[string]interface{}, key string) (string, bool) {
	v, ok := payload[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}
func asString(v interface{}) string  { s, _ := v.(string); return s }
func pipelineARN(name string) string { return fmt.Sprintf(pipelineARNFormat, name) }
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"__type": code, "message": message})
}
