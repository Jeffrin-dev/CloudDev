package waf

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type WebACL struct {
	Id            string                 `json:"Id"`
	Name          string                 `json:"Name"`
	ARN           string                 `json:"ARN"`
	Scope         string                 `json:"Scope"`
	DefaultAction map[string]interface{} `json:"DefaultAction"`
	Rules         []Rule                 `json:"Rules"`
}

type Rule struct {
	Name     string                 `json:"Name"`
	Priority int                    `json:"Priority"`
	Action   map[string]interface{} `json:"Action"`
}

type RuleGroup struct {
	Id    string `json:"Id"`
	Name  string `json:"Name"`
	ARN   string `json:"ARN"`
	Scope string `json:"Scope"`
}

type server struct {
	mu           sync.RWMutex
	webACLs      map[string]WebACL
	ruleGroups   map[string]RuleGroup
	associations map[string]string
}

func newServer() *server {
	return &server{
		webACLs:      make(map[string]WebACL),
		ruleGroups:   make(map[string]RuleGroup),
		associations: make(map[string]string),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}
	target := r.Header.Get("X-Amz-Target")
	switch target {
	case "AWSWAF_20190729.CreateWebACL":
		s.handleCreateWebACL(w, r)
	case "AWSWAF_20190729.DeleteWebACL":
		s.handleDeleteWebACL(w, r)
	case "AWSWAF_20190729.GetWebACL":
		s.handleGetWebACL(w, r)
	case "AWSWAF_20190729.ListWebACLs":
		s.handleListWebACLs(w)
	case "AWSWAF_20190729.CreateRuleGroup":
		s.handleCreateRuleGroup(w, r)
	case "AWSWAF_20190729.ListRuleGroups":
		s.handleListRuleGroups(w)
	case "AWSWAF_20190729.AssociateWebACL":
		s.handleAssociateWebACL(w, r)
	case "AWSWAF_20190729.DisassociateWebACL":
		s.handleDisassociateWebACL(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "unknown target"})
	}
}

func (s *server) handleCreateWebACL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string                 `json:"Name"`
		Scope         string                 `json:"Scope"`
		DefaultAction map[string]interface{} `json:"DefaultAction"`
		Rules         []Rule                 `json:"Rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	id := uuidString()
	acl := WebACL{
		Id:            id,
		Name:          req.Name,
		ARN:           fmt.Sprintf("arn:aws:wafv2:us-east-1:000000000000:regional/webacl/%s/%s", req.Name, id),
		Scope:         req.Scope,
		DefaultAction: req.DefaultAction,
		Rules:         req.Rules,
	}
	s.mu.Lock()
	s.webACLs[id] = acl
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Summary": map[string]string{"Id": acl.Id, "Name": acl.Name, "ARN": acl.ARN},
	})
}

func (s *server) handleDeleteWebACL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id string `json:"Id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	s.mu.Lock()
	delete(s.webACLs, req.Id)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (s *server) handleGetWebACL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id string `json:"Id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	s.mu.RLock()
	acl, ok := s.webACLs[req.Id]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "web acl not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"WebACL": acl})
}

func (s *server) handleListWebACLs(w http.ResponseWriter) {
	s.mu.RLock()
	summaries := make([]map[string]string, 0, len(s.webACLs))
	for _, acl := range s.webACLs {
		summaries = append(summaries, map[string]string{"Id": acl.Id, "Name": acl.Name, "ARN": acl.ARN})
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"WebACLs": summaries})
}

func (s *server) handleCreateRuleGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"Name"`
		Scope    string `json:"Scope"`
		Capacity int    `json:"Capacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	_ = req.Capacity
	id := uuidString()
	rg := RuleGroup{Id: id, Name: req.Name, Scope: req.Scope, ARN: fmt.Sprintf("arn:aws:wafv2:us-east-1:000000000000:regional/rulegroup/%s/%s", req.Name, id)}
	s.mu.Lock()
	s.ruleGroups[id] = rg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"Summary": rg})
}

func (s *server) handleListRuleGroups(w http.ResponseWriter) {
	s.mu.RLock()
	items := make([]RuleGroup, 0, len(s.ruleGroups))
	for _, rg := range s.ruleGroups {
		items = append(items, rg)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"RuleGroups": items})
}

func (s *server) handleAssociateWebACL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WebACLArn   string `json:"WebACLArn"`
		ResourceArn string `json:"ResourceArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	s.mu.Lock()
	s.associations[req.ResourceArn] = req.WebACLArn
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (s *server) handleDisassociateWebACL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string `json:"ResourceArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	s.mu.Lock()
	delete(s.associations, req.ResourceArn)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func uuidString() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}
