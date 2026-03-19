package secretsmanager

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

type Secret struct {
	Name         string `json:"Name"`
	SecretString string `json:"SecretString,omitempty"`
	ARN          string `json:"ARN"`
}

type server struct {
	mu      sync.RWMutex
	secrets map[string]Secret
}

func newServer() *server {
	return &server{secrets: make(map[string]Secret)}
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
	case "secretsmanager.CreateSecret":
		s.handleCreateSecret(w, payload)
	case "secretsmanager.GetSecretValue":
		s.handleGetSecretValue(w, payload)
	case "secretsmanager.DeleteSecret":
		s.handleDeleteSecret(w, payload)
	case "secretsmanager.ListSecrets":
		s.handleListSecrets(w)
	case "secretsmanager.UpdateSecret":
		s.handleUpdateSecret(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateSecret(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Name is required")
		return
	}
	secretString, ok := stringField(payload, "SecretString")
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "SecretString is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.secrets[name]; exists {
		writeError(w, http.StatusBadRequest, "ResourceExistsException", "Secret already exists")
		return
	}

	secret := Secret{Name: name, SecretString: secretString, ARN: secretARN(name)}
	s.secrets[name] = secret
	writeJSON(w, http.StatusOK, secret)
}

func (s *server) handleGetSecretValue(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "SecretId")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "SecretId is required")
		return
	}

	s.mu.RLock()
	secret, exists := s.secrets[name]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Secret not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ARN":          secret.ARN,
		"Name":         secret.Name,
		"SecretString": secret.SecretString,
	})
}

func (s *server) handleDeleteSecret(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "SecretId")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "SecretId is required")
		return
	}

	s.mu.Lock()
	secret, exists := s.secrets[name]
	if exists {
		delete(s.secrets, name)
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Secret not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ARN":          secret.ARN,
		"Name":         secret.Name,
		"DeletionDate": "now",
	})
}

func (s *server) handleListSecrets(w http.ResponseWriter) {
	s.mu.RLock()
	secrets := make([]Secret, 0, len(s.secrets))
	for _, secret := range s.secrets {
		secrets = append(secrets, secret)
	}
	s.mu.RUnlock()

	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })
	writeJSON(w, http.StatusOK, map[string]interface{}{"SecretList": secrets})
}

func (s *server) handleUpdateSecret(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "SecretId")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "SecretId is required")
		return
	}
	secretString, ok := stringField(payload, "SecretString")
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "SecretString is required")
		return
	}

	s.mu.Lock()
	secret, exists := s.secrets[name]
	if exists {
		secret.SecretString = secretString
		s.secrets[name] = secret
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Secret not found")
		return
	}

	writeJSON(w, http.StatusOK, secret)
}

func secretARN(name string) string {
	return fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:%s", region, accountID, name)
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
