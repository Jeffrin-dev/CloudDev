package appsync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type GraphqlApi struct {
	ApiId              string            `json:"ApiId"`
	Name               string            `json:"Name"`
	AuthenticationType string            `json:"AuthenticationType"`
	Uris               map[string]string `json:"Uris"`
}

type DataSource struct {
	Name           string `json:"Name"`
	Type           string `json:"Type"`
	ServiceRoleArn string `json:"ServiceRoleArn"`
}

type server struct {
	mu          sync.RWMutex
	nextAPIID   int
	apis        map[string]GraphqlApi
	datasources map[string][]DataSource
	schemas     map[string]string
}

func newServer() *server {
	return &server{apis: make(map[string]GraphqlApi), datasources: make(map[string][]DataSource), schemas: make(map[string]string)}
}

func Start(port int) error { return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer()) }

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	switch {
	case len(parts) == 2 && parts[0] == "v1" && parts[1] == "apis":
		s.handleApisRoot(w, r)
	case len(parts) == 3 && parts[0] == "v1" && parts[1] == "apis":
		s.handleApiByID(w, r, parts[2])
	case len(parts) == 4 && parts[0] == "v1" && parts[1] == "apis":
		s.handleApiChildCollection(w, r, parts[2], parts[3])
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
}

func (s *server) handleApisRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.createGraphqlAPI(w, r)
		return
	}
	if r.Method == http.MethodGet {
		s.listGraphqlAPIs(w)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
}

func (s *server) handleApiByID(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method == http.MethodGet {
		s.getGraphqlAPI(w, apiID)
		return
	}
	if r.Method == http.MethodDelete {
		s.deleteGraphqlAPI(w, apiID)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
}

func (s *server) handleApiChildCollection(w http.ResponseWriter, r *http.Request, apiID, child string) {
	switch child {
	case "datasources":
		if r.Method == http.MethodPost {
			s.createDataSource(w, r, apiID)
			return
		}
		if r.Method == http.MethodGet {
			s.listDataSources(w, apiID)
			return
		}
	case "schemacreation":
		if r.Method == http.MethodPost {
			s.startSchemaCreation(w, r, apiID)
			return
		}
	case "schema":
		if r.Method == http.MethodGet {
			s.getIntrospectionSchema(w, apiID)
			return
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
}

func (s *server) createGraphqlAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name               string `json:"name"`
		AuthenticationType string `json:"authenticationType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "name is required"})
		return
	}
	s.mu.Lock()
	s.nextAPIID++
	apiID := fmt.Sprintf("api-%d", s.nextAPIID)
	api := GraphqlApi{ApiId: apiID, Name: req.Name, AuthenticationType: req.AuthenticationType, Uris: map[string]string{"GRAPHQL": fmt.Sprintf("http://localhost:4567/graphql/%s", apiID)}}
	s.apis[apiID] = api
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, api)
}

func (s *server) listGraphqlAPIs(w http.ResponseWriter) {
	s.mu.RLock()
	items := make([]GraphqlApi, 0, len(s.apis))
	for _, api := range s.apis {
		items = append(items, api)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"graphqlApis": items})
}

func (s *server) getGraphqlAPI(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	api, ok := s.apis[apiID]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	writeJSON(w, http.StatusOK, api)
}

func (s *server) deleteGraphqlAPI(w http.ResponseWriter, apiID string) {
	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	delete(s.apis, apiID)
	delete(s.datasources, apiID)
	delete(s.schemas, apiID)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"message": "Deleted"})
}

func (s *server) createDataSource(w http.ResponseWriter, r *http.Request, apiID string) {
	var req DataSource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Type) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Name and Type are required"})
		return
	}
	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	s.datasources[apiID] = append(s.datasources[apiID], req)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, req)
}

func (s *server) listDataSources(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.RUnlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	items := append([]DataSource(nil), s.datasources[apiID]...)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"dataSources": items})
}

func (s *server) startSchemaCreation(w http.ResponseWriter, r *http.Request, apiID string) {
	var req struct {
		Definition string `json:"definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	s.schemas[apiID] = req.Definition
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "PROCESSING"})
}

func (s *server) getIntrospectionSchema(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.RUnlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	schema := s.schemas[apiID]
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]string{"schema": schema})
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
