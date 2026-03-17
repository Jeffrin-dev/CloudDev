package lambda

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

const functionsBasePath = "/2015-03-31/functions"

type LambdaFunction struct {
	Name        string
	Runtime     string
	Handler     string
	Code        []byte
	Environment map[string]string
}

type server struct {
	mu        sync.RWMutex
	functions map[string]LambdaFunction
}

type createFunctionRequest struct {
	FunctionName string `json:"FunctionName"`
	Runtime      string `json:"Runtime"`
	Handler      string `json:"Handler"`
	Code         struct {
		ZipFile string `json:"ZipFile"`
	} `json:"Code"`
}

type functionResponse struct {
	FunctionName string `json:"FunctionName"`
	Runtime      string `json:"Runtime,omitempty"`
	Handler      string `json:"Handler,omitempty"`
	FunctionArn  string `json:"FunctionArn"`
	State        string `json:"State"`
}

func newServer() *server {
	return &server{functions: make(map[string]LambdaFunction)}
}

func Start(port int, functionsDir string, hotReload bool) error {
	_ = functionsDir
	_ = hotReload

	srv := newServer()
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == functionsBasePath || r.URL.Path == functionsBasePath+"/" {
		s.handleFunctionsRoot(w, r)
		return
	}

	if !hasPrefix(r.URL.Path, functionsBasePath+"/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	remainder := r.URL.Path[len(functionsBasePath)+1:]
	if hasSuffix(remainder, "/invocations") {
		name := remainder[:len(remainder)-len("/invocations")]
		if name == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
			return
		}
		s.invokeFunction(w, r, name)
		return
	}

	if remainder == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
		return
	}
	s.deleteFunction(w, remainder)
}

func (s *server) handleFunctionsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFunctions(w)
	case http.MethodPost:
		s.createFunction(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *server) listFunctions(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]string, 0, len(s.functions))
	for name := range s.functions {
		names = append(names, name)
	}
	sort.Strings(names)

	functions := make([]functionResponse, 0, len(names))
	for _, name := range names {
		fn := s.functions[name]
		functions = append(functions, functionResponse{
			FunctionName: fn.Name,
			Runtime:      fn.Runtime,
			Handler:      fn.Handler,
			FunctionArn:  functionARN(fn.Name),
			State:        "Active",
		})
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Functions": functions})
}

func (s *server) createFunction(w http.ResponseWriter, r *http.Request) {
	var req createFunctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}

	if req.FunctionName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "FunctionName is required"})
		return
	}

	fn := LambdaFunction{
		Name:        req.FunctionName,
		Runtime:     req.Runtime,
		Handler:     req.Handler,
		Code:        []byte(req.Code.ZipFile),
		Environment: map[string]string{},
	}

	s.mu.Lock()
	s.functions[req.FunctionName] = fn
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, functionResponse{
		FunctionName: fn.Name,
		Runtime:      fn.Runtime,
		Handler:      fn.Handler,
		FunctionArn:  functionARN(fn.Name),
		State:        "Active",
	})
}

func (s *server) invokeFunction(w http.ResponseWriter, r *http.Request, functionName string) {
	s.mu.RLock()
	_, exists := s.functions[functionName]
	s.mu.RUnlock()
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Function not found"})
		return
	}

	var payload any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"statusCode":   200,
		"body":         "Function executed successfully",
		"functionName": functionName,
		"payload":      payload,
	})
}

func (s *server) deleteFunction(w http.ResponseWriter, functionName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.functions[functionName]; !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Function not found"})
		return
	}
	delete(s.functions, functionName)
	w.WriteHeader(http.StatusNoContent)
}

func functionARN(name string) string {
	return "arn:aws:lambda:us-east-1:000000000000:function:" + name
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
