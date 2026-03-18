package lambda

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateListInvokeAndDeleteFunction(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createBody := map[string]any{
		"FunctionName": "hello",
		"Runtime":      "python3.9",
		"Handler":      "hello.handler",
		"Role":         "arn:...",
		"Code": map[string]any{
			"ZipFile": "dGVzdA==",
		},
	}

	resp := doJSONRequest(t, http.MethodPost, srv.URL+functionsBasePath, createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	createResp := decodeJSONBody(t, resp)
	if createResp["FunctionName"] != "hello" {
		t.Fatalf("expected function name hello, got %v", createResp["FunctionName"])
	}
	if createResp["FunctionArn"] != "arn:aws:lambda:us-east-1:000000000000:function:hello" {
		t.Fatalf("unexpected arn: %v", createResp["FunctionArn"])
	}

	listResp, err := http.Get(srv.URL + functionsBasePath)
	if err != nil {
		t.Fatalf("list functions failed: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	listBody := decodeJSONBody(t, listResp)
	fns, ok := listBody["Functions"].([]any)
	if !ok || len(fns) != 1 {
		t.Fatalf("expected one function, got %#v", listBody["Functions"])
	}

	invokeBody := map[string]any{"message": "hi"}
	invokeResp := doJSONRequest(t, http.MethodPost, srv.URL+functionsBasePath+"/hello/invocations", invokeBody)
	if invokeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", invokeResp.StatusCode)
	}
	invoked := decodeJSONBody(t, invokeResp)
	if invoked["functionName"] != "hello" {
		t.Fatalf("expected functionName hello, got %v", invoked["functionName"])
	}
	if invoked["body"] != "Function executed successfully" {
		t.Fatalf("unexpected body: %v", invoked["body"])
	}
	payload, ok := invoked["payload"].(map[string]any)
	if !ok || payload["message"] != "hi" {
		t.Fatalf("unexpected payload: %#v", invoked["payload"])
	}

	delReq, err := http.NewRequest(http.MethodDelete, srv.URL+functionsBasePath+"/hello", nil)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete function failed: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	invokeMissing := doJSONRequest(t, http.MethodPost, srv.URL+functionsBasePath+"/hello/invocations", invokeBody)
	if invokeMissing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing function, got %d", invokeMissing.StatusCode)
	}
}

func TestCreateFunctionRequiresName(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	resp := doJSONRequest(t, http.MethodPost, srv.URL+functionsBasePath, map[string]any{"Runtime": "go1.x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHotReloadAutoLoadsNewZipFile(t *testing.T) {
	t.Parallel()
	functionsDir := t.TempDir()
	srv := newServer()

	go srv.watchFunctionsDir(functionsDir, 20*time.Millisecond)

	zipPath := filepath.Join(functionsDir, "processor.zip")
	if err := os.WriteFile(zipPath, []byte("zip-content"), 0o644); err != nil {
		t.Fatalf("write zip file: %v", err)
	}

	waitForFunction(t, srv, "processor", true)

	srv.mu.RLock()
	fn, ok := srv.functions["processor"]
	srv.mu.RUnlock()
	if !ok {
		t.Fatalf("expected processor function to be auto-loaded")
	}
	if string(fn.Code) != "zip-content" {
		t.Fatalf("expected zip content to be loaded, got %q", string(fn.Code))
	}
}

func TestHotReloadDisabledDoesNotAutoLoad(t *testing.T) {
	t.Parallel()
	functionsDir := t.TempDir()
	srv := newServer()

	zipPath := filepath.Join(functionsDir, "disabled.zip")
	if err := os.WriteFile(zipPath, []byte("zip-content"), 0o644); err != nil {
		t.Fatalf("write zip file: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	srv.mu.RLock()
	_, ok := srv.functions["disabled"]
	srv.mu.RUnlock()
	if ok {
		t.Fatalf("expected disabled function not to be auto-loaded when hot reload is off")
	}
}

func TestEnsureFunctionsDirCreatesDirectory(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	functionsDir := filepath.Join(base, "functions")

	if err := ensureFunctionsDir(functionsDir); err != nil {
		t.Fatalf("ensure functions dir: %v", err)
	}

	info, err := os.Stat(functionsDir)
	if err != nil {
		t.Fatalf("stat functions dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", functionsDir)
	}
}

func waitForFunction(t *testing.T, srv *server, functionName string, expected bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.RLock()
		_, ok := srv.functions[functionName]
		srv.mu.RUnlock()
		if ok == expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for function %s existence=%v", functionName, expected)
}

func doJSONRequest(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSONBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return payload
}
