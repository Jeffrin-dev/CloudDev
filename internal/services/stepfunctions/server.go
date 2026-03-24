package stepfunctions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.0"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

type StateMachine struct {
	StateMachineArn string
	Name            string
	Definition      string
	Status          string
}

type Execution struct {
	ExecutionArn    string
	StateMachineArn string
	Status          string
	Input           string
	Output          string
}

type server struct {
	mu              sync.RWMutex
	stateMachines   map[string]StateMachine
	executions      map[string]Execution
	nextExecutionID int
}

func newServer() *server {
	return &server{
		stateMachines: make(map[string]StateMachine),
		executions:    make(map[string]Execution),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

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
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Invalid JSON body")
		return
	}

	switch target {
	case "AWSStepFunctions.CreateStateMachine", "AmazonStates.CreateStateMachine":
		s.handleCreateStateMachine(w, payload)
	case "AWSStepFunctions.DeleteStateMachine", "AmazonStates.DeleteStateMachine":
		s.handleDeleteStateMachine(w, payload)
	case "AWSStepFunctions.ListStateMachines", "AmazonStates.ListStateMachines":
		s.handleListStateMachines(w)
	case "AWSStepFunctions.DescribeStateMachine", "AmazonStates.DescribeStateMachine":
		s.handleDescribeStateMachine(w, payload)
	case "AWSStepFunctions.StartExecution", "AmazonStates.StartExecution":
		s.handleStartExecution(w, payload)
	case "AWSStepFunctions.StopExecution", "AmazonStates.StopExecution":
		s.handleStopExecution(w, payload)
	case "AWSStepFunctions.ListExecutions", "AmazonStates.ListExecutions":
		s.handleListExecutions(w, payload)
	case "AWSStepFunctions.DescribeExecution", "AmazonStates.DescribeExecution":
		s.handleDescribeExecution(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateStateMachine(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "name is required")
		return
	}
	definition, ok := stringField(payload, "definition")
	if !ok || definition == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "definition is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	arn := stateMachineARN(name)
	if _, exists := s.stateMachines[arn]; exists {
		writeError(w, http.StatusBadRequest, "StateMachineAlreadyExists", "State machine already exists")
		return
	}
	stateMachine := StateMachine{
		StateMachineArn: arn,
		Name:            name,
		Definition:      definition,
		Status:          "ACTIVE",
	}
	s.stateMachines[arn] = stateMachine

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stateMachineArn": arn,
		"creationDate":    "now",
	})
}

func (s *server) handleDeleteStateMachine(w http.ResponseWriter, payload map[string]interface{}) {
	arn, ok := stringField(payload, "stateMachineArn")
	if !ok || arn == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "stateMachineArn is required")
		return
	}

	s.mu.Lock()
	_, exists := s.stateMachines[arn]
	if exists {
		delete(s.stateMachines, arn)
	}
	for executionArn, execution := range s.executions {
		if execution.StateMachineArn == arn {
			delete(s.executions, executionArn)
		}
	}
	s.mu.Unlock()

	if !exists {
		writeError(w, http.StatusBadRequest, "StateMachineDoesNotExist", "State machine does not exist")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleListStateMachines(w http.ResponseWriter) {
	s.mu.RLock()
	stateMachines := make([]StateMachine, 0, len(s.stateMachines))
	for _, sm := range s.stateMachines {
		stateMachines = append(stateMachines, sm)
	}
	s.mu.RUnlock()

	sort.Slice(stateMachines, func(i, j int) bool {
		return stateMachines[i].Name < stateMachines[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"stateMachines": stateMachines})
}

func (s *server) handleDescribeStateMachine(w http.ResponseWriter, payload map[string]interface{}) {
	arn, ok := stringField(payload, "stateMachineArn")
	if !ok || arn == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "stateMachineArn is required")
		return
	}

	s.mu.RLock()
	stateMachine, exists := s.stateMachines[arn]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "StateMachineDoesNotExist", "State machine does not exist")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stateMachineArn": stateMachine.StateMachineArn,
		"name":            stateMachine.Name,
		"definition":      stateMachine.Definition,
		"status":          stateMachine.Status,
	})
}

func (s *server) handleStartExecution(w http.ResponseWriter, payload map[string]interface{}) {
	stateMachineArn, ok := stringField(payload, "stateMachineArn")
	if !ok || stateMachineArn == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "stateMachineArn is required")
		return
	}
	input, _ := stringField(payload, "input")

	s.mu.Lock()
	if _, exists := s.stateMachines[stateMachineArn]; !exists {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "StateMachineDoesNotExist", "State machine does not exist")
		return
	}
	s.nextExecutionID++
	executionArn := fmt.Sprintf("%s:execution:%d", stateMachineArn, s.nextExecutionID)
	execution := Execution{
		ExecutionArn:    executionArn,
		StateMachineArn: stateMachineArn,
		Status:          "RUNNING",
		Input:           input,
	}
	execution.Status = "SUCCEEDED"
	execution.Output = input
	s.executions[executionArn] = execution
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"executionArn": executionArn,
		"startDate":    "now",
	})
}

func (s *server) handleStopExecution(w http.ResponseWriter, payload map[string]interface{}) {
	executionArn, ok := stringField(payload, "executionArn")
	if !ok || executionArn == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "executionArn is required")
		return
	}

	s.mu.Lock()
	execution, exists := s.executions[executionArn]
	if exists {
		execution.Status = "ABORTED"
		s.executions[executionArn] = execution
	}
	s.mu.Unlock()

	if !exists {
		writeError(w, http.StatusBadRequest, "ExecutionDoesNotExist", "Execution does not exist")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stopDate": "now",
	})
}

func (s *server) handleListExecutions(w http.ResponseWriter, payload map[string]interface{}) {
	stateMachineArn, ok := stringField(payload, "stateMachineArn")
	if !ok || stateMachineArn == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "stateMachineArn is required")
		return
	}

	s.mu.RLock()
	executions := make([]Execution, 0)
	for _, execution := range s.executions {
		if execution.StateMachineArn == stateMachineArn {
			executions = append(executions, execution)
		}
	}
	s.mu.RUnlock()

	sort.Slice(executions, func(i, j int) bool {
		return executions[i].ExecutionArn < executions[j].ExecutionArn
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"executions": executions})
}

func (s *server) handleDescribeExecution(w http.ResponseWriter, payload map[string]interface{}) {
	executionArn, ok := stringField(payload, "executionArn")
	if !ok || executionArn == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "executionArn is required")
		return
	}

	s.mu.RLock()
	execution, exists := s.executions[executionArn]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ExecutionDoesNotExist", "Execution does not exist")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"executionArn":    execution.ExecutionArn,
		"stateMachineArn": execution.StateMachineArn,
		"status":          execution.Status,
		"input":           execution.Input,
		"output":          execution.Output,
	})
}

func stateMachineARN(name string) string {
	return fmt.Sprintf("arn:aws:states:%s:%s:stateMachine:%s", region, accountID, name)
}

func stringField(payload map[string]interface{}, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	if !ok {
		return "", false
	}
	return str, true
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"__type":  code,
		"message": message,
	})
}
