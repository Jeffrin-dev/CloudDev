package apigatewayv2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type Api struct {
	ApiId        string `json:"ApiId"`
	Name         string `json:"Name"`
	ProtocolType string `json:"ProtocolType"`
	ApiEndpoint  string `json:"ApiEndpoint"`
}

type Route struct {
	RouteId  string `json:"RouteId"`
	RouteKey string `json:"RouteKey"`
	Target   string `json:"Target"`
}

type Integration struct {
	IntegrationId   string `json:"IntegrationId"`
	IntegrationType string `json:"IntegrationType"`
	IntegrationUri  string `json:"IntegrationUri"`
}

type Stage struct {
	StageName string `json:"StageName"`
}

type createApiRequest struct {
	Name         string `json:"Name"`
	ProtocolType string `json:"ProtocolType"`
}

type createRouteRequest struct {
	RouteKey string `json:"RouteKey"`
	Target   string `json:"Target"`
}

type createIntegrationRequest struct {
	IntegrationType string `json:"IntegrationType"`
	IntegrationUri  string `json:"IntegrationUri"`
}

type createStageRequest struct {
	StageName string `json:"StageName"`
}

type server struct {
	mu              sync.RWMutex
	nextAPIID       int
	nextRouteID     int
	nextIntegration int
	apis            map[string]Api
	routes          map[string][]Route
	integrations    map[string][]Integration
	stages          map[string][]Stage
}

func newServer() *server {
	return &server{
		apis:         make(map[string]Api),
		routes:       make(map[string][]Route),
		integrations: make(map[string][]Integration),
		stages:       make(map[string][]Stage),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) == 2 && parts[0] == "v2" && parts[1] == "apis" {
		s.handleApisRoot(w, r)
		return
	}

	if len(parts) == 3 && parts[0] == "v2" && parts[1] == "apis" {
		s.handleApiByID(w, r, parts[2])
		return
	}

	if len(parts) == 4 && parts[0] == "v2" && parts[1] == "apis" {
		s.handleApiChildCollection(w, r, parts[2], parts[3])
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
}

func (s *server) handleApisRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createAPI(w, r)
	case http.MethodGet:
		s.getAPIs(w)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *server) handleApiByID(w http.ResponseWriter, r *http.Request, apiID string) {
	switch r.Method {
	case http.MethodGet:
		s.getAPI(w, apiID)
	case http.MethodDelete:
		s.deleteAPI(w, apiID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *server) handleApiChildCollection(w http.ResponseWriter, r *http.Request, apiID string, child string) {
	switch child {
	case "routes":
		switch r.Method {
		case http.MethodPost:
			s.createRoute(w, r, apiID)
		case http.MethodGet:
			s.getRoutes(w, apiID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
		}
	case "integrations":
		switch r.Method {
		case http.MethodPost:
			s.createIntegration(w, r, apiID)
		case http.MethodGet:
			s.getIntegrations(w, apiID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
		}
	case "stages":
		switch r.Method {
		case http.MethodPost:
			s.createStage(w, r, apiID)
		case http.MethodGet:
			s.getStages(w, apiID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
}

func (s *server) createAPI(w http.ResponseWriter, r *http.Request) {
	var req createApiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Name is required"})
		return
	}
	if strings.TrimSpace(req.ProtocolType) == "" {
		req.ProtocolType = "HTTP"
	}

	s.mu.Lock()
	s.nextAPIID++
	apiID := fmt.Sprintf("api-%d", s.nextAPIID)
	api := Api{
		ApiId:        apiID,
		Name:         req.Name,
		ProtocolType: req.ProtocolType,
		ApiEndpoint:  fmt.Sprintf("http://localhost:4573/%s", apiID),
	}
	s.apis[apiID] = api
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, api)
}

func (s *server) getAPIs(w http.ResponseWriter) {
	s.mu.RLock()
	items := make([]Api, 0, len(s.apis))
	for _, api := range s.apis {
		items = append(items, api)
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Items": items})
}

func (s *server) getAPI(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	api, ok := s.apis[apiID]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	writeJSON(w, http.StatusOK, api)
}

func (s *server) deleteAPI(w http.ResponseWriter, apiID string) {
	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	delete(s.apis, apiID)
	delete(s.routes, apiID)
	delete(s.integrations, apiID)
	delete(s.stages, apiID)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"message": "Deleted"})
}

func (s *server) createRoute(w http.ResponseWriter, r *http.Request, apiID string) {
	var req createRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.RouteKey) == "" || strings.TrimSpace(req.Target) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "RouteKey and Target are required"})
		return
	}

	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	s.nextRouteID++
	route := Route{RouteId: fmt.Sprintf("route-%d", s.nextRouteID), RouteKey: req.RouteKey, Target: req.Target}
	s.routes[apiID] = append(s.routes[apiID], route)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, route)
}

func (s *server) getRoutes(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.RUnlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	items := append([]Route(nil), s.routes[apiID]...)
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Items": items})
}

func (s *server) createIntegration(w http.ResponseWriter, r *http.Request, apiID string) {
	var req createIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.IntegrationType) == "" || strings.TrimSpace(req.IntegrationUri) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "IntegrationType and IntegrationUri are required"})
		return
	}

	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	s.nextIntegration++
	integration := Integration{
		IntegrationId:   fmt.Sprintf("int-%d", s.nextIntegration),
		IntegrationType: req.IntegrationType,
		IntegrationUri:  req.IntegrationUri,
	}
	s.integrations[apiID] = append(s.integrations[apiID], integration)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, integration)
}

func (s *server) getIntegrations(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.RUnlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	items := append([]Integration(nil), s.integrations[apiID]...)
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Items": items})
}

func (s *server) createStage(w http.ResponseWriter, r *http.Request, apiID string) {
	var req createStageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.StageName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "StageName is required"})
		return
	}

	s.mu.Lock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	stage := Stage{StageName: req.StageName}
	s.stages[apiID] = append(s.stages[apiID], stage)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, stage)
}

func (s *server) getStages(w http.ResponseWriter, apiID string) {
	s.mu.RLock()
	if _, ok := s.apis[apiID]; !ok {
		s.mu.RUnlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	items := append([]Stage(nil), s.stages[apiID]...)
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Items": items})
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
