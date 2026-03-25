package ssm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

var supportedTypes = map[string]struct{}{
	"String":       {},
	"StringList":   {},
	"SecureString": {},
}

type Parameter struct {
	Name    string `json:"Name"`
	Value   string `json:"Value,omitempty"`
	Type    string `json:"Type"`
	Version int    `json:"Version"`
	ARN     string `json:"ARN"`
}

type server struct {
	mu         sync.RWMutex
	parameters map[string]Parameter
}

func newServer() *server {
	return &server{parameters: make(map[string]Parameter)}
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
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Invalid JSON body")
		return
	}

	switch target {
	case "AmazonSSM.PutParameter":
		s.handlePutParameter(w, payload)
	case "AmazonSSM.GetParameter":
		s.handleGetParameter(w, payload)
	case "AmazonSSM.GetParameters":
		s.handleGetParameters(w, payload)
	case "AmazonSSM.GetParametersByPath":
		s.handleGetParametersByPath(w, payload)
	case "AmazonSSM.DeleteParameter":
		s.handleDeleteParameter(w, payload)
	case "AmazonSSM.DescribeParameters":
		s.handleDescribeParameters(w)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handlePutParameter(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}
	value, ok := stringField(payload, "Value")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Value is required")
		return
	}
	paramType, ok := stringField(payload, "Type")
	if !ok || paramType == "" {
		paramType = "String"
	}
	if _, valid := supportedTypes[paramType]; !valid {
		writeError(w, http.StatusBadRequest, "ValidationException", "Unsupported parameter type")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	version := 1
	if existing, exists := s.parameters[name]; exists {
		version = existing.Version + 1
	}
	param := Parameter{
		Name:    name,
		Value:   value,
		Type:    paramType,
		Version: version,
		ARN:     parameterARN(name),
	}
	s.parameters[name] = param

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Version": version,
		"Tier":    "Standard",
	})
}

func (s *server) handleGetParameter(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}

	s.mu.RLock()
	param, exists := s.parameters[name]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ParameterNotFound", "Parameter not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"Parameter": param})
}

func (s *server) handleGetParameters(w http.ResponseWriter, payload map[string]interface{}) {
	names, ok := stringSliceField(payload, "Names")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Names is required")
		return
	}

	s.mu.RLock()
	parameters := make([]Parameter, 0, len(names))
	invalid := make([]string, 0)
	for _, name := range names {
		if param, exists := s.parameters[name]; exists {
			parameters = append(parameters, param)
		} else {
			invalid = append(invalid, name)
		}
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Parameters":        parameters,
		"InvalidParameters": invalid,
	})
}

func (s *server) handleGetParametersByPath(w http.ResponseWriter, payload map[string]interface{}) {
	path, ok := stringField(payload, "Path")
	if !ok || path == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Path is required")
		return
	}

	s.mu.RLock()
	parameters := make([]Parameter, 0)
	for _, param := range s.parameters {
		if strings.HasPrefix(param.Name, path) {
			parameters = append(parameters, param)
		}
	}
	s.mu.RUnlock()

	sort.Slice(parameters, func(i, j int) bool {
		return parameters[i].Name < parameters[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"Parameters": parameters})
}

func (s *server) handleDeleteParameter(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}

	s.mu.Lock()
	_, exists := s.parameters[name]
	if exists {
		delete(s.parameters, name)
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ParameterNotFound", "Parameter not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleDescribeParameters(w http.ResponseWriter) {
	s.mu.RLock()
	metadata := make([]Parameter, 0, len(s.parameters))
	for _, param := range s.parameters {
		metadata = append(metadata, Parameter{
			Name:    param.Name,
			Type:    param.Type,
			Version: param.Version,
			ARN:     param.ARN,
		})
	}
	s.mu.RUnlock()

	sort.Slice(metadata, func(i, j int) bool {
		return metadata[i].Name < metadata[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"Parameters": metadata})
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

func stringSliceField(payload map[string]interface{}, key string) ([]string, bool) {
	raw, ok := payload[key]
	if !ok {
		return nil, false
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		str, ok := item.(string)
		if !ok {
			return nil, false
		}
		values = append(values, str)
	}
	return values, true
}

func parameterARN(name string) string {
	return fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", region, accountID, name)
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
