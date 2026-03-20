package kms

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateKey(t *testing.T) {
	srv := newServer()
	resp := performJSONRequest(t, srv, "TrentService.CreateKey", map[string]any{"Description": "test key"})
	metadata := resp["KeyMetadata"].(map[string]any)
	if metadata["Description"] != "test key" {
		t.Fatalf("expected key description test key, got %v", metadata["Description"])
	}
	if metadata["KeyId"] == "" {
		t.Fatal("expected key id to be set")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	srv := newServer()
	createResp := performJSONRequest(t, srv, "TrentService.CreateKey", map[string]any{"Description": "roundtrip key"})
	keyID := createResp["KeyMetadata"].(map[string]any)["KeyId"].(string)

	encryptResp := performJSONRequest(t, srv, "TrentService.Encrypt", map[string]any{"KeyId": keyID, "Plaintext": "hello world"})
	ciphertext := encryptResp["CiphertextBlob"].(string)
	if ciphertext == "" {
		t.Fatal("expected ciphertext blob")
	}

	decryptResp := performJSONRequest(t, srv, "TrentService.Decrypt", map[string]any{"CiphertextBlob": ciphertext})
	if decryptResp["Plaintext"] != "hello world" {
		t.Fatalf("expected plaintext hello world, got %v", decryptResp["Plaintext"])
	}
}

func performJSONRequest(t *testing.T, handler http.Handler, target string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("Content-Type", jsonContentType)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d with body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}
