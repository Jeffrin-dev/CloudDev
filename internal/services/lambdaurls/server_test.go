package lambdaurls

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateFunctionURLConfig(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4595, 4574))
	t.Cleanup(srv.Close)

	resp := doJSONRequest(t, http.MethodPost, srv.URL+"/2021-10-31/functions/hello/url", map[string]any{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var cfg FunctionUrlConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	resp.Body.Close()

	if cfg.FunctionArn != "arn:aws:lambda:us-east-1:000000000000:function:hello" {
		t.Fatalf("unexpected function arn: %s", cfg.FunctionArn)
	}
	if cfg.FunctionUrl != "http://localhost:4595/function-url/hello/" {
		t.Fatalf("unexpected function url: %s", cfg.FunctionUrl)
	}
	if cfg.AuthType != "NONE" {
		t.Fatalf("expected default auth type NONE, got %s", cfg.AuthType)
	}
	if cfg.CreatedTime == "" {
		t.Fatalf("expected created time to be set")
	}
}

func TestGetFunctionURLConfig(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4595, 4574))
	t.Cleanup(srv.Close)

	doJSONRequest(t, http.MethodPost, srv.URL+"/2021-10-31/functions/hello/url", map[string]any{"AuthType": "NONE"}).Body.Close()

	resp, err := http.Get(srv.URL + "/2021-10-31/functions/hello/url")
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg FunctionUrlConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if cfg.FunctionArn != "arn:aws:lambda:us-east-1:000000000000:function:hello" {
		t.Fatalf("unexpected function arn: %s", cfg.FunctionArn)
	}
	if cfg.FunctionUrl != "http://localhost:4595/function-url/hello/" {
		t.Fatalf("unexpected function url: %s", cfg.FunctionUrl)
	}
}

func doJSONRequest(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}
