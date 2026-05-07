package waf

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndListWebACLs(t *testing.T) {
	s := newServer()

	createBody := `{"Name":"acl-1","Scope":"REGIONAL","DefaultAction":{"Allow":{}},"Rules":[{"Name":"r1","Priority":1,"Action":{"Allow":{}}}]}`
	createReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(createBody))
	createReq.Header.Set("X-Amz-Target", "AWSWAF_20190729.CreateWebACL")
	createRec := httptest.NewRecorder()
	s.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", createRec.Code)
	}

	var created struct {
		Summary struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
			ARN  string `json:"ARN"`
		} `json:"Summary"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Summary.Id == "" || created.Summary.ARN == "" || created.Summary.Name != "acl-1" {
		t.Fatalf("unexpected summary: %+v", created.Summary)
	}

	listReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	listReq.Header.Set("X-Amz-Target", "AWSWAF_20190729.ListWebACLs")
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}
	var listed struct {
		WebACLs []struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
			ARN  string `json:"ARN"`
		} `json:"WebACLs"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.WebACLs) != 1 {
		t.Fatalf("expected 1 webacl, got %d", len(listed.WebACLs))
	}
}

func TestCreateRuleGroup(t *testing.T) {
	s := newServer()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"Name":"rg-1","Scope":"REGIONAL","Capacity":10}`))
	req.Header.Set("X-Amz-Target", "AWSWAF_20190729.CreateRuleGroup")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Summary RuleGroup `json:"Summary"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Summary.Id == "" || resp.Summary.Name != "rg-1" || resp.Summary.ARN == "" {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
}

func TestAssociateWebACL(t *testing.T) {
	s := newServer()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"WebACLArn":"arn:aws:wafv2:us-east-1:000000000000:regional/webacl/acl-1/abc","ResourceArn":"arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/test/123"}`))
	req.Header.Set("X-Amz-Target", "AWSWAF_20190729.AssociateWebACL")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	s.mu.RLock()
	got, ok := s.associations["arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/test/123"]
	s.mu.RUnlock()
	if !ok || got == "" {
		t.Fatalf("expected association to be stored")
	}
}
