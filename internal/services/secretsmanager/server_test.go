package secretsmanager

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateGetUpdateDeleteAndListSecrets(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createResp := doSecretsRequest(t, srv.URL, "secretsmanager.CreateSecret", map[string]interface{}{
		"Name":         "db-password",
		"SecretString": "initial",
	})
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected create status 200, got %d", createResp.StatusCode)
	}
	assertContentType(t, createResp)
	var created Secret
	decodeResponse(t, createResp.Body, &created)
	if created.ARN != "arn:aws:secretsmanager:us-east-1:000000000000:secret:db-password" {
		t.Fatalf("unexpected ARN: %q", created.ARN)
	}

	getResp := doSecretsRequest(t, srv.URL, "secretsmanager.GetSecretValue", map[string]interface{}{"SecretId": "db-password"})
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected get status 200, got %d", getResp.StatusCode)
	}
	var got map[string]interface{}
	decodeResponse(t, getResp.Body, &got)
	if got["SecretString"] != "initial" {
		t.Fatalf("unexpected secret value: %#v", got)
	}

	updateResp := doSecretsRequest(t, srv.URL, "secretsmanager.UpdateSecret", map[string]interface{}{
		"SecretId":     "db-password",
		"SecretString": "rotated",
	})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updateResp.StatusCode)
	}
	var updated Secret
	decodeResponse(t, updateResp.Body, &updated)
	if updated.SecretString != "rotated" {
		t.Fatalf("unexpected updated secret: %#v", updated)
	}

	listResp := doSecretsRequest(t, srv.URL, "secretsmanager.ListSecrets", map[string]interface{}{})
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.StatusCode)
	}
	var listed map[string][]Secret
	decodeResponse(t, listResp.Body, &listed)
	if len(listed["SecretList"]) != 1 || listed["SecretList"][0].Name != "db-password" {
		t.Fatalf("unexpected list response: %#v", listed)
	}

	deleteResp := doSecretsRequest(t, srv.URL, "secretsmanager.DeleteSecret", map[string]interface{}{"SecretId": "db-password"})
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d", deleteResp.StatusCode)
	}
	var deleted map[string]interface{}
	decodeResponse(t, deleteResp.Body, &deleted)
	if deleted["Name"] != "db-password" {
		t.Fatalf("unexpected delete response: %#v", deleted)
	}

	missingResp := doSecretsRequest(t, srv.URL, "secretsmanager.GetSecretValue", map[string]interface{}{"SecretId": "db-password"})
	defer missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing secret status 400, got %d", missingResp.StatusCode)
	}
}

func doSecretsRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("Content-Type", jsonContentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func decodeResponse(t *testing.T, r io.ReadCloser, v interface{}) {
	t.Helper()
	defer r.Close()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertContentType(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		resp.Body.Close()
		t.Fatalf("expected Content-Type %q, got %q", jsonContentType, got)
	}
}
