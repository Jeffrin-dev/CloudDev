package lambda

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const jsonContentType = "application/json"

// LambdaFunction represents a registered lambda function.
type LambdaFunction struct {
	Name        string            `json:"name"`
	Runtime     string            `json:"runtime"`
	Handler     string            `json:"handler"`
	Code        []byte            `json:"-"`
	Environment map[string]string `json:"environment,omitempty"`
}

type server struct {
	mu           sync.RWMutex
	functions    map[string]LambdaFunction
	functionsDir string
	hotReload    bool
}

func newServer(functionsDir string, hotReload bool) *server {
	return &server{
		functions:    make(map[string]LambdaFunction),
		functionsDir: functionsDir,
		hotReload:    hotReload,
	}
}

// Start starts the Lambda-compatible HTTP server.
func Start(port int, functionsDir string, hotReload bool) error {
	srv := newServer(functionsDir, hotReload)
	srv.loadFunctionsFromDir()
	if hotReload {
		go srv.watchFunctionsDir()
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "2015-03-31" || parts[1] != "functions" {
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		s.handleListFunctions(w)
	case len(parts) == 2 && r.Method == http.MethodPost:
		s.handleCreateFunction(w, r)
	case len(parts) == 4 && parts[3] == "invocations" && r.Method == http.MethodPost:
		s.handleInvokeFunction(w, r, parts[2])
	case len(parts) == 3 && r.Method == http.MethodDelete:
		s.handleDeleteFunction(w, parts[2])
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "Method not allowed")
	}
}

func (s *server) handleCreateFunction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FunctionName string            `json:"FunctionName"`
		Runtime      string            `json:"Runtime"`
		Handler      string            `json:"Handler"`
		Role         string            `json:"Role"`
		Code         struct {
			ZipFile string `json:"ZipFile"`
		} `json:"Code"`
		Code         []byte            `json:"Code"`
		Environment  map[string]string `json:"Environment"`
	}
	if err := decodeBody(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "Invalid request body")
		return
	}
	if req.FunctionName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "FunctionName is required")
		return
	}

	var code []byte
	if req.Code.ZipFile != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.Code.ZipFile)
		if err != nil {
			writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "Invalid request body")
			return
		}
		code = decoded
	if req.Runtime == "" {
		req.Runtime = "provided.al2"
	}
	if req.Handler == "" {
		req.Handler = "handler"
	}

	s.mu.Lock()
	s.functions[req.FunctionName] = LambdaFunction{
		Name:        req.FunctionName,
		Runtime:     req.Runtime,
		Handler:     req.Handler,
		Code:        code,
		Code:        req.Code,
		Environment: req.Environment,
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"FunctionName": req.FunctionName,
		"Runtime":      req.Runtime,
		"Handler":      req.Handler,
		"FunctionArn":  fmt.Sprintf("arn:aws:lambda:us-east-1:000000000000:function:%s", req.FunctionName),
		"State":        "Active",
	})
}

func (s *server) handleListFunctions(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]string, 0, len(s.functions))
	for name := range s.functions {
		names = append(names, name)
	}
	s.mu.RUnlock()
	sort.Strings(names)

	functions := make([]map[string]interface{}, 0, len(names))
	s.mu.RLock()
	for _, name := range names {
		fn := s.functions[name]
		functions = append(functions, map[string]interface{}{
			"FunctionName": fn.Name,
			"Runtime":      fn.Runtime,
			"Handler":      fn.Handler,
			"State":        "Active",
		})
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"Functions": functions})
}

func (s *server) handleInvokeFunction(w http.ResponseWriter, r *http.Request, functionName string) {
	s.mu.RLock()
	_, exists := s.functions[functionName]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Function not found")
		return
	}

	payload := json.RawMessage([]byte("{}"))
	if body, err := io.ReadAll(r.Body); err == nil && len(strings.TrimSpace(string(body))) > 0 {
		payload = json.RawMessage(body)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"statusCode":   200,
		"body":         "Function executed successfully",
		"functionName": functionName,
		"payload":      payload,
	})
}

func (s *server) handleDeleteFunction(w http.ResponseWriter, functionName string) {
	s.mu.Lock()
	_, exists := s.functions[functionName]
	if exists {
		delete(s.functions, functionName)
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Function not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"Message": "Function deleted"})
}

func (s *server) watchFunctionsDir() {
	if s.functionsDir == "" {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.loadFunctionsFromDir()
	}
}

func (s *server) loadFunctionsFromDir() {
	if s.functionsDir == "" {
		return
	}
	entries, err := os.ReadDir(s.functionsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(s.functionsDir, entry.Name())
		code, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		s.mu.Lock()
		fn := s.functions[name]
		if fn.Name == "" {
			fn = LambdaFunction{Name: name, Runtime: "provided.al2", Handler: "handler", Environment: map[string]string{}}
		}
		fn.Code = code
		s.functions[name] = fn
		s.mu.Unlock()
	}
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

func decodeBody(body io.Reader, out interface{}) error {
	decoder := json.NewDecoder(body)
	return decoder.Decode(out)
}
