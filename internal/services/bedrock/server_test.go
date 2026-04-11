package bedrock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListFoundationModels(t *testing.T) {
	h := newServer()
	req := httptest.NewRequest(http.MethodPost, "/foundation-models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	summaries, ok := resp["modelSummaries"].([]any)
	if !ok {
		t.Fatalf("expected modelSummaries array, got %T", resp["modelSummaries"])
	}
	if len(summaries) != 4 {
		t.Fatalf("expected 4 models, got %d", len(summaries))
	}
}

func TestInvokeModelClaude(t *testing.T) {
	h := newServer()
	payload := map[string]any{
		"messages":   []map[string]any{{"role": "user", "content": "hello"}},
		"max_tokens": 10,
	}
	resp := performInvoke(t, h, "/model/anthropic.claude-3-sonnet-20240229-v1:0/invoke", payload)

	content := resp["content"].([]any)
	first := content[0].(map[string]any)
	if first["text"] != "Mock response from Claude: " {
		t.Fatalf("expected mock claude text, got %v", first["text"])
	}
	if resp["stop_reason"] != "end_turn" {
		t.Fatalf("expected end_turn, got %v", resp["stop_reason"])
	}
}

func TestInvokeModelTitan(t *testing.T) {
	h := newServer()
	payload := map[string]any{"inputText": "hello"}
	resp := performInvoke(t, h, "/model/amazon.titan-text-express-v1/invoke", payload)

	results := resp["results"].([]any)
	first := results[0].(map[string]any)
	if first["outputText"] != "Mock response from Titan: " {
		t.Fatalf("expected mock titan text, got %v", first["outputText"])
	}
	if first["completionReason"] != "FINISH" {
		t.Fatalf("expected FINISH, got %v", first["completionReason"])
	}
}

func performInvoke(t *testing.T, h http.Handler, path string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
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
