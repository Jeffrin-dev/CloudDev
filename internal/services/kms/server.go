package kms

import (
	"crypto/rand"
	"encoding/base64"
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

type Key struct {
	KeyId       string
	KeyArn      string
	Description string
	Enabled     bool
}

type server struct {
	mu      sync.RWMutex
	keys    map[string]Key
	nextKey int
}

func newServer() *server {
	return &server{keys: make(map[string]Key)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "InvalidAction", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "MissingHeaderException", "Missing X-Amz-Target header")
		return
	}

	payload := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "SerializationException", "Invalid JSON body")
		return
	}

	switch target {
	case "TrentService.CreateKey":
		s.createKey(w, payload)
	case "TrentService.ListKeys":
		s.listKeys(w)
	case "TrentService.DescribeKey":
		s.describeKey(w, payload)
	case "TrentService.Encrypt":
		s.encrypt(w, payload)
	case "TrentService.Decrypt":
		s.decrypt(w, payload)
	case "TrentService.GenerateDataKey":
		s.generateDataKey(w, payload)
	case "TrentService.ScheduleKeyDeletion":
		s.scheduleKeyDeletion(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) createKey(w http.ResponseWriter, payload map[string]any) {
	description, _ := stringField(payload, "Description")

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextKey++
	keyID := fmt.Sprintf("key-%d", s.nextKey)
	key := Key{KeyId: keyID, KeyArn: keyARN(keyID), Description: description, Enabled: true}
	s.keys[keyID] = key
	writeJSON(w, http.StatusOK, map[string]any{"KeyMetadata": keyMetadata(key)})
}

func (s *server) listKeys(w http.ResponseWriter) {
	s.mu.RLock()
	keys := make([]Key, 0, len(s.keys))
	for _, key := range s.keys {
		keys = append(keys, key)
	}
	s.mu.RUnlock()
	sort.Slice(keys, func(i, j int) bool { return keys[i].KeyId < keys[j].KeyId })

	entries := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, map[string]any{"KeyArn": key.KeyArn, "KeyId": key.KeyId})
	}
	writeJSON(w, http.StatusOK, map[string]any{"Keys": entries})
}

func (s *server) describeKey(w http.ResponseWriter, payload map[string]any) {
	key, ok := s.lookupKey(payload)
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "Key not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"KeyMetadata": keyMetadata(key)})
}

func (s *server) encrypt(w http.ResponseWriter, payload map[string]any) {
	key, ok := s.lookupKey(payload)
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "Key not found")
		return
	}
	plaintext, ok := stringField(payload, "Plaintext")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Plaintext is required")
		return
	}
	ciphertext := base64.StdEncoding.EncodeToString([]byte(key.KeyId + ":" + plaintext))
	writeJSON(w, http.StatusOK, map[string]any{"CiphertextBlob": ciphertext, "KeyId": key.KeyArn})
}

func (s *server) decrypt(w http.ResponseWriter, payload map[string]any) {
	ciphertext, ok := stringField(payload, "CiphertextBlob")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "CiphertextBlob is required")
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidCiphertextException", "CiphertextBlob is invalid")
		return
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "InvalidCiphertextException", "CiphertextBlob is invalid")
		return
	}
	keyID := parts[0]

	s.mu.RLock()
	key, ok := s.keys[keyID]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "Key not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"KeyId": key.KeyArn, "Plaintext": parts[1]})
}

func (s *server) generateDataKey(w http.ResponseWriter, payload map[string]any) {
	key, ok := s.lookupKey(payload)
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "Key not found")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, "InternalFailure", "Could not generate data key")
		return
	}
	plaintext := base64.StdEncoding.EncodeToString(raw)
	ciphertext := base64.StdEncoding.EncodeToString([]byte(key.KeyId + ":" + plaintext))
	writeJSON(w, http.StatusOK, map[string]any{"CiphertextBlob": ciphertext, "KeyId": key.KeyArn, "Plaintext": plaintext})
}

func (s *server) scheduleKeyDeletion(w http.ResponseWriter, payload map[string]any) {
	key, ok := s.lookupKey(payload)
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "Key not found")
		return
	}
	s.mu.Lock()
	stored := s.keys[key.KeyId]
	stored.Enabled = false
	s.keys[key.KeyId] = stored
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"KeyId": key.KeyId, "DeletionDate": "scheduled"})
}

func (s *server) lookupKey(payload map[string]any) (Key, bool) {
	keyID, ok := stringField(payload, "KeyId")
	if !ok || keyID == "" {
		return Key{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if key, ok := s.keys[keyID]; ok {
		return key, true
	}
	for _, key := range s.keys {
		if key.KeyArn == keyID {
			return key, true
		}
	}
	return Key{}, false
}

func stringField(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

func keyMetadata(key Key) map[string]any {
	return map[string]any{
		"Arn":         key.KeyArn,
		"Description": key.Description,
		"Enabled":     key.Enabled,
		"KeyId":       key.KeyId,
	}
}

func keyARN(keyID string) string {
	return fmt.Sprintf("arn:aws:kms:%s:%s:key/%s", region, accountID, keyID)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"__type": code, "message": message})
}
