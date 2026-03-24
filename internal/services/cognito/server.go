package cognito

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

type UserPool struct {
	Id     string
	Name   string
	Status string
	Arn    string
}

type UserPoolClient struct {
	ClientId   string
	ClientName string
	UserPoolId string
}

type User struct {
	Username   string
	UserPoolId string
	Enabled    bool
	Status     string
	Attributes []UserAttribute
}

type UserAttribute struct {
	Name  string
	Value string
}

type server struct {
	mu           sync.RWMutex
	pools        map[string]UserPool
	poolClients  map[string]map[string]UserPoolClient
	usersByPool  map[string]map[string]User
	nextPoolID   int
	nextClientID int
}

func newServer() *server {
	return &server{
		pools:       make(map[string]UserPool),
		poolClients: make(map[string]map[string]UserPoolClient),
		usersByPool: make(map[string]map[string]User),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "InvalidParameterException", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Missing X-Amz-Target header")
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Invalid JSON body")
		return
	}

	switch target {
	case "AWSCognitoIdentityProviderService.CreateUserPool":
		s.createUserPool(w, payload)
	case "AWSCognitoIdentityProviderService.DeleteUserPool":
		s.deleteUserPool(w, payload)
	case "AWSCognitoIdentityProviderService.ListUserPools":
		s.listUserPools(w)
	case "AWSCognitoIdentityProviderService.DescribeUserPool":
		s.describeUserPool(w, payload)
	case "AWSCognitoIdentityProviderService.CreateUserPoolClient":
		s.createUserPoolClient(w, payload)
	case "AWSCognitoIdentityProviderService.ListUserPoolClients":
		s.listUserPoolClients(w, payload)
	case "AWSCognitoIdentityProviderService.AdminCreateUser":
		s.adminCreateUser(w, payload)
	case "AWSCognitoIdentityProviderService.AdminDeleteUser":
		s.adminDeleteUser(w, payload)
	case "AWSCognitoIdentityProviderService.ListUsers":
		s.listUsers(w, payload)
	case "AWSCognitoIdentityProviderService.InitiateAuth":
		s.initiateAuth(w)
	case "AWSCognitoIdentityProviderService.RespondToAuthChallenge":
		s.respondToAuthChallenge(w)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) createUserPool(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "PoolName")
	if !ok || strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "PoolName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextPoolID++
	id := fmt.Sprintf("local_%06d", s.nextPoolID)
	pool := UserPool{
		Id:     id,
		Name:   name,
		Status: "ACTIVE",
		Arn:    userPoolARN(id),
	}
	s.pools[id] = pool

	writeJSON(w, http.StatusOK, map[string]any{"UserPool": pool})
}

func (s *server) deleteUserPool(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}

	s.mu.Lock()
	delete(s.pools, poolID)
	delete(s.poolClients, poolID)
	delete(s.usersByPool, poolID)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) listUserPools(w http.ResponseWriter) {
	s.mu.RLock()
	pools := make([]UserPool, 0, len(s.pools))
	for _, pool := range s.pools {
		pools = append(pools, pool)
	}
	s.mu.RUnlock()

	sort.Slice(pools, func(i, j int) bool { return pools[i].Id < pools[j].Id })
	writeJSON(w, http.StatusOK, map[string]any{"UserPools": pools})
}

func (s *server) describeUserPool(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}

	s.mu.RLock()
	pool, exists := s.pools[poolID]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "User pool not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"UserPool": pool})
}

func (s *server) createUserPoolClient(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}
	clientName, ok := stringField(payload, "ClientName")
	if !ok || strings.TrimSpace(clientName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "ClientName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pools[poolID]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "User pool not found")
		return
	}
	s.nextClientID++
	clientID := fmt.Sprintf("client_%06d", s.nextClientID)
	client := UserPoolClient{ClientId: clientID, ClientName: clientName, UserPoolId: poolID}
	if s.poolClients[poolID] == nil {
		s.poolClients[poolID] = make(map[string]UserPoolClient)
	}
	s.poolClients[poolID][clientID] = client

	writeJSON(w, http.StatusOK, map[string]any{"UserPoolClient": client})
}

func (s *server) listUserPoolClients(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}

	s.mu.RLock()
	clientsMap := s.poolClients[poolID]
	clients := make([]UserPoolClient, 0, len(clientsMap))
	for _, client := range clientsMap {
		clients = append(clients, client)
	}
	s.mu.RUnlock()

	sort.Slice(clients, func(i, j int) bool { return clients[i].ClientId < clients[j].ClientId })
	writeJSON(w, http.StatusOK, map[string]any{"UserPoolClients": clients})
}

func (s *server) adminCreateUser(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}
	username, ok := stringField(payload, "Username")
	if !ok || strings.TrimSpace(username) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Username is required")
		return
	}

	attributes := parseAttributes(payload["UserAttributes"])

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pools[poolID]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "User pool not found")
		return
	}
	if s.usersByPool[poolID] == nil {
		s.usersByPool[poolID] = make(map[string]User)
	}

	user := User{
		Username:   username,
		UserPoolId: poolID,
		Enabled:    true,
		Status:     "CONFIRMED",
		Attributes: attributes,
	}
	s.usersByPool[poolID][username] = user
	writeJSON(w, http.StatusOK, map[string]any{"User": user})
}

func (s *server) adminDeleteUser(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}
	username, ok := stringField(payload, "Username")
	if !ok || username == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Username is required")
		return
	}

	s.mu.Lock()
	if users := s.usersByPool[poolID]; users != nil {
		delete(users, username)
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) listUsers(w http.ResponseWriter, payload map[string]any) {
	poolID, ok := stringField(payload, "UserPoolId")
	if !ok || poolID == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "UserPoolId is required")
		return
	}

	s.mu.RLock()
	userMap := s.usersByPool[poolID]
	users := make([]User, 0, len(userMap))
	for _, user := range userMap {
		users = append(users, user)
	}
	s.mu.RUnlock()

	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	writeJSON(w, http.StatusOK, map[string]any{"Users": users})
}

func (s *server) initiateAuth(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, authResponse())
}

func (s *server) respondToAuthChallenge(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, authResponse())
}

func authResponse() map[string]any {
	return map[string]any{
		"AuthenticationResult": map[string]any{
			"IdToken":      "mock-id-token",
			"AccessToken":  "mock-access-token",
			"RefreshToken": "mock-refresh-token",
			"ExpiresIn":    3600,
		},
	}
}

func parseAttributes(v any) []UserAttribute {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]UserAttribute, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["Name"].(string)
		value, _ := m["Value"].(string)
		if name == "" {
			continue
		}
		result = append(result, UserAttribute{Name: name, Value: value})
	}
	return result
}

func userPoolARN(poolID string) string {
	return fmt.Sprintf("arn:aws:cognito-idp:%s:%s:userpool/%s", region, accountID, poolID)
}

func stringField(payload map[string]any, key string) (string, bool) {
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"__type":  code,
		"message": message,
	})
}
