package opensearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type Domain struct {
	DomainName    string `json:"DomainName"`
	DomainId      string `json:"DomainId"`
	ARN           string `json:"ARN"`
	Endpoint      string `json:"Endpoint"`
	EngineVersion string `json:"EngineVersion"`
	Created       bool   `json:"Created"`
	Deleted       bool   `json:"Deleted"`
}

type Tag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

type server struct {
	mu      sync.RWMutex
	domains map[string]Domain
	tags    map[string][]Tag
}

func newServer() *server {
	return &server{
		domains: make(map[string]Domain),
		tags:    make(map[string][]Tag),
	}
}

func Start(port int) error {
	srv := newServer()
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 || parts[0] != "2021-01-01" || parts[1] != "opensearch" || parts[2] != "domain" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	switch {
	case len(parts) == 3 && r.Method == http.MethodPost:
		s.createDomain(w, r)
	case len(parts) == 3 && r.Method == http.MethodGet:
		s.listDomainNames(w)
	case len(parts) == 4 && r.Method == http.MethodGet:
		s.describeDomain(w, parts[3])
	case len(parts) == 4 && r.Method == http.MethodDelete:
		s.deleteDomain(w, parts[3])
	case len(parts) == 5 && parts[4] == "tags" && r.Method == http.MethodPost:
		s.addTags(w, r)
	case len(parts) == 5 && parts[4] == "tags" && r.Method == http.MethodGet:
		s.listTags(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) createDomain(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		DomainName    string `json:"DomainName"`
		EngineVersion string `json:"EngineVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.DomainName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request"})
		return
	}
	domainName := strings.TrimSpace(req.DomainName)
	domain := Domain{
		DomainName:    domainName,
		DomainId:      "000000000000/" + domainName,
		ARN:           "arn:aws:es:us-east-1:000000000000:domain/" + domainName,
		Endpoint:      domainName + ".us-east-1.es.localhost",
		EngineVersion: strings.TrimSpace(req.EngineVersion),
		Created:       true,
	}

	s.mu.Lock()
	s.domains[domainName] = domain
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]Domain{"DomainStatus": domain})
}

func (s *server) listDomainNames(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]map[string]string, 0, len(s.domains))
	for _, domain := range s.domains {
		names = append(names, map[string]string{"DomainName": domain.DomainName})
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"DomainNames": names})
}

func (s *server) describeDomain(w http.ResponseWriter, domainName string) {
	s.mu.RLock()
	domain, ok := s.domains[domainName]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Domain not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]Domain{"DomainStatus": domain})
}

func (s *server) deleteDomain(w http.ResponseWriter, domainName string) {
	s.mu.Lock()
	domain, ok := s.domains[domainName]
	if ok {
		domain.Deleted = true
		s.domains[domainName] = domain
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Domain not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]Domain{"DomainStatus": domain})
}

func (s *server) addTags(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		ARN     string `json:"ARN"`
		TagList []Tag  `json:"TagList"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ARN) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request"})
		return
	}
	s.mu.Lock()
	s.tags[req.ARN] = append([]Tag(nil), req.TagList...)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (s *server) listTags(w http.ResponseWriter, r *http.Request) {
	arn := strings.TrimSpace(r.URL.Query().Get("arn"))
	s.mu.RLock()
	tags := append([]Tag(nil), s.tags[arn]...)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"TagList": tags})
}
