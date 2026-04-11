package lambdaurls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const functionURLBasePath = "/2021-10-31/functions"

type CorsConfig struct {
	AllowCredentials bool     `json:"AllowCredentials,omitempty"`
	AllowHeaders     []string `json:"AllowHeaders,omitempty"`
	AllowMethods     []string `json:"AllowMethods,omitempty"`
	AllowOrigins     []string `json:"AllowOrigins,omitempty"`
	ExposeHeaders    []string `json:"ExposeHeaders,omitempty"`
	MaxAge           int      `json:"MaxAge,omitempty"`
}

type FunctionUrlConfig struct {
	FunctionArn string     `json:"FunctionArn"`
	FunctionUrl string     `json:"FunctionUrl"`
	AuthType    string     `json:"AuthType"`
	Cors        CorsConfig `json:"Cors"`
	CreatedTime string     `json:"CreatedTime"`
}

type server struct {
	mu         sync.RWMutex
	port       int
	lambdaPort int
	configs    map[string]FunctionUrlConfig
}

type upsertFunctionURLConfigRequest struct {
	AuthType string     `json:"AuthType"`
	Cors     CorsConfig `json:"Cors"`
}

func newServer(port int, lambdaPort int) *server {
	return &server{
		port:       port,
		lambdaPort: lambdaPort,
		configs:    make(map[string]FunctionUrlConfig),
	}
}

func Start(port int, lambdaPort int) error {
	srv := newServer(port, lambdaPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/function-url/") {
		s.handleFunctionURLInvocation(w, r)
		return
	}

	parts := splitPath(r.URL.Path)
	if len(parts) < 4 || parts[0] != "2021-10-31" || parts[1] != "functions" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	functionName := parts[2]
	resource := parts[3]
	if functionName == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	switch {
	case len(parts) == 4 && resource == "url" && r.Method == http.MethodPost:
		s.createFunctionURLConfig(w, r, functionName)
	case len(parts) == 4 && resource == "url" && r.Method == http.MethodGet:
		s.getFunctionURLConfig(w, functionName)
	case len(parts) == 4 && resource == "url" && r.Method == http.MethodPut:
		s.updateFunctionURLConfig(w, r, functionName)
	case len(parts) == 4 && resource == "url" && r.Method == http.MethodDelete:
		s.deleteFunctionURLConfig(w, functionName)
	case len(parts) == 4 && resource == "urls" && r.Method == http.MethodGet:
		s.listFunctionURLConfigs(w, functionName)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *server) createFunctionURLConfig(w http.ResponseWriter, r *http.Request, functionName string) {
	cfgReq, ok := decodeUpsertRequest(w, r)
	if !ok {
		return
	}

	authType := strings.TrimSpace(cfgReq.AuthType)
	if authType == "" {
		authType = "NONE"
	}

	cfg := FunctionUrlConfig{
		FunctionArn: functionARN(functionName),
		FunctionUrl: functionURL(s.port, functionName),
		AuthType:    authType,
		Cors:        cfgReq.Cors,
		CreatedTime: time.Now().UTC().Format(time.RFC3339),
	}

	s.mu.Lock()
	s.configs[functionName] = cfg
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, cfg)
}

func (s *server) getFunctionURLConfig(w http.ResponseWriter, functionName string) {
	s.mu.RLock()
	cfg, exists := s.configs[functionName]
	s.mu.RUnlock()
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Function URL config not found"})
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

func (s *server) updateFunctionURLConfig(w http.ResponseWriter, r *http.Request, functionName string) {
	cfgReq, ok := decodeUpsertRequest(w, r)
	if !ok {
		return
	}

	s.mu.Lock()
	cfg, exists := s.configs[functionName]
	if !exists {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Function URL config not found"})
		return
	}
	if strings.TrimSpace(cfgReq.AuthType) != "" {
		cfg.AuthType = cfgReq.AuthType
	}
	cfg.Cors = cfgReq.Cors
	s.configs[functionName] = cfg
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, cfg)
}

func (s *server) deleteFunctionURLConfig(w http.ResponseWriter, functionName string) {
	s.mu.Lock()
	_, exists := s.configs[functionName]
	if exists {
		delete(s.configs, functionName)
	}
	s.mu.Unlock()

	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Function URL config not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listFunctionURLConfigs(w http.ResponseWriter, functionName string) {
	s.mu.RLock()
	cfg, exists := s.configs[functionName]
	s.mu.RUnlock()

	configs := make([]FunctionUrlConfig, 0, 1)
	if exists {
		configs = append(configs, cfg)
	}
	writeJSON(w, http.StatusOK, map[string]any{"FunctionUrlConfigs": configs})
}

func (s *server) handleFunctionURLInvocation(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 2 || parts[0] != "function-url" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	functionName := parts[1]
	s.mu.RLock()
	_, exists := s.configs[functionName]
	s.mu.RUnlock()
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Function URL config not found"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}

	invokeURL := fmt.Sprintf("http://localhost:%d/2015-03-31/functions/%s/invocations", s.lambdaPort, functionName)
	invokeReq, err := http.NewRequest(http.MethodPost, invokeURL, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to build invoke request"})
		return
	}
	invokeReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(invokeReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "Failed to invoke Lambda function"})
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "Failed to read Lambda response"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func decodeUpsertRequest(w http.ResponseWriter, r *http.Request) (upsertFunctionURLConfigRequest, bool) {
	defer r.Body.Close()
	var req upsertFunctionURLConfigRequest
	if r.Body == nil {
		return req, true
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return upsertFunctionURLConfigRequest{}, false
	}
	return req, true
}

func functionARN(name string) string {
	return "arn:aws:lambda:us-east-1:000000000000:function:" + name
}

func functionURL(port int, functionName string) string {
	return fmt.Sprintf("http://localhost:%d/function-url/%s/", port, functionName)
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
