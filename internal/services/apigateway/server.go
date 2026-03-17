package apigateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type API struct {
	Id          string
	Name        string
	Description string
	Routes      map[string]Route
}

type Route struct {
	Path           string
	Method         string
	LambdaFunction string
	LambdaPort     int
}

type deployment struct {
	Id      string
	Stage   string
	APIId   string
	Created string
}

type server struct {
	mu          sync.RWMutex
	lambdaPort  int
	nextAPIID   int
	nextDeploy  int
	apis        map[string]API
	deployments map[string][]deployment
}

type createAPIRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createResourceRequest struct {
	Path           string `json:"path"`
	Method         string `json:"method"`
	LambdaFunction string `json:"lambdaFunction"`
	LambdaPort     int    `json:"lambdaPort"`
}

type createDeploymentRequest struct {
	StageName string `json:"stageName"`
}

func newServer(lambdaPort int) *server {
	return &server{
		lambdaPort:  lambdaPort,
		apis:        make(map[string]API),
		deployments: make(map[string][]deployment),
	}
}

func Start(port int, lambdaPort int) error {
	srv := newServer(lambdaPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/restapis" || r.URL.Path == "/restapis/" {
		s.handleRestAPIsRoot(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/restapis/") {
		s.handleRestAPIChild(w, r)
		return
	}

	s.handleProxyInvocation(w, r)
}

func (s *server) handleRestAPIsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createAPI(w, r)
	case http.MethodGet:
		s.listAPIs(w)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *server) handleRestAPIChild(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) != 3 || parts[0] != "restapis" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	apiID := parts[1]
	if parts[2] != "resources" && parts[2] != "deployments" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	if parts[2] == "resources" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
			return
		}
		s.createResource(w, r, apiID)
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
		return
	}
	s.deployAPI(w, r, apiID)
}

func (s *server) createAPI(w http.ResponseWriter, r *http.Request) {
	var req createAPIRequest
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
	id := fmt.Sprintf("api-%d", s.nextAPIID)
	api := API{Id: id, Name: req.Name, Description: req.Description, Routes: make(map[string]Route)}
	s.apis[id] = api
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          api.Id,
		"name":        api.Name,
		"description": api.Description,
	})
}

func (s *server) listAPIs(w http.ResponseWriter) {
	s.mu.RLock()
	items := make([]map[string]any, 0, len(s.apis))
	for _, api := range s.apis {
		items = append(items, map[string]any{
			"id":          api.Id,
			"name":        api.Name,
			"description": api.Description,
		})
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"item": items})
}

func (s *server) createResource(w http.ResponseWriter, r *http.Request, apiID string) {
	var req createResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Method) == "" || strings.TrimSpace(req.LambdaFunction) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "path, method and lambdaFunction are required"})
		return
	}

	method := strings.ToUpper(req.Method)
	path := req.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	port := req.LambdaPort
	if port == 0 {
		port = s.lambdaPort
	}

	s.mu.Lock()
	api, exists := s.apis[apiID]
	if !exists {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	key := routeKey(method, path)
	route := Route{Path: path, Method: method, LambdaFunction: req.LambdaFunction, LambdaPort: port}
	api.Routes[key] = route
	s.apis[apiID] = api
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":             apiID + ":" + key,
		"path":           route.Path,
		"httpMethod":     route.Method,
		"lambdaFunction": route.LambdaFunction,
		"lambdaPort":     route.LambdaPort,
	})
}

func (s *server) deployAPI(w http.ResponseWriter, r *http.Request, apiID string) {
	var req createDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.StageName) == "" {
		req.StageName = "prod"
	}

	s.mu.Lock()
	if _, exists := s.apis[apiID]; !exists {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	s.nextDeploy++
	d := deployment{Id: fmt.Sprintf("dep-%d", s.nextDeploy), Stage: req.StageName, APIId: apiID, Created: "now"}
	s.deployments[apiID] = append(s.deployments[apiID], d)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":                d.Id,
		"description":       "",
		"createdDate":       d.Created,
		"stageName":         d.Stage,
		"restApiId":         d.APIId,
		"invokeUrlTemplate": fmt.Sprintf("http://localhost/{apiId}/{stage}/%s", "{proxy+}"),
	})
}

func (s *server) handleProxyInvocation(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	apiID := parts[0]
	proxyPath := "/" + strings.Join(parts[2:], "/")
	method := strings.ToUpper(r.Method)

	s.mu.RLock()
	api, exists := s.apis[apiID]
	if !exists {
		s.mu.RUnlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "API not found"})
		return
	}
	route, ok := api.Routes[routeKey(method, proxyPath)]
	if !ok {
		route, ok = api.Routes[routeKey("ANY", proxyPath)]
	}
	s.mu.RUnlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Route not found"})
		return
	}

	targetURL := fmt.Sprintf("http://localhost:%d/2015-03-31/functions/%s/invocations", route.LambdaPort, route.LambdaFunction)
	forwardReq, err := http.NewRequest(http.MethodPost, targetURL, r.Body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to create invocation request"})
		return
	}
	forwardReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(forwardReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "Failed to invoke Lambda"})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "Lambda returned invalid JSON"})
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func routeKey(method string, path string) string {
	return strings.ToUpper(method) + " " + path
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
