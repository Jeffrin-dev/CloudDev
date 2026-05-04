package acm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestDescribeListAndGetCertificate(t *testing.T) {
	srv := newServer()

	requestResp := doRequest(t, srv, "CertificateManager.RequestCertificate", map[string]any{
		"DomainName": "example.com",
		"SubjectAlternativeNames": []string{"www.example.com"},
	})
	arn, _ := requestResp["CertificateArn"].(string)
	if !strings.HasPrefix(arn, "arn:aws:acm:us-east-1:000000000000:certificate/") {
		t.Fatalf("unexpected arn: %s", arn)
	}

	describeResp := doRequest(t, srv, "CertificateManager.DescribeCertificate", map[string]any{"CertificateArn": arn})
	cert := describeResp["Certificate"].(map[string]any)
	if cert["DomainName"] != "example.com" {
		t.Fatalf("expected domain example.com, got %v", cert["DomainName"])
	}
	if cert["Status"] != "ISSUED" {
		t.Fatalf("expected status ISSUED, got %v", cert["Status"])
	}

	listResp := doRequest(t, srv, "CertificateManager.ListCertificates", map[string]any{})
	list := listResp["CertificateSummaryList"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 certificate in list, got %d", len(list))
	}

	getResp := doRequest(t, srv, "CertificateManager.GetCertificate", map[string]any{"CertificateArn": arn})
	if !strings.Contains(getResp["Certificate"].(string), "BEGIN CERTIFICATE") {
		t.Fatalf("expected mock certificate PEM, got %v", getResp["Certificate"])
	}
	if !strings.Contains(getResp["CertificateChain"].(string), "BEGIN CERTIFICATE") {
		t.Fatalf("expected mock certificate chain PEM, got %v", getResp["CertificateChain"])
	}
}

func doRequest(t *testing.T, handler http.Handler, target string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Amz-Target", target)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}
	out := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}
