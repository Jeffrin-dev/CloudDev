package lambdalayers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishLayerVersion(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	resp := doJSONRequest(t, http.MethodPost, srv.URL+"/2018-11-14/layers/utils/versions", map[string]any{
		"Description":        "Common utility layer",
		"CompatibleRuntimes": []string{"go1.x", "nodejs20.x"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	payload := decodeJSONBody(t, resp)
	if payload["Version"].(float64) != 1 {
		t.Fatalf("expected version 1, got %v", payload["Version"])
	}
	if payload["LayerArn"] != "arn:aws:lambda:us-east-1:000000000000:layer:utils:1" {
		t.Fatalf("unexpected layer arn: %v", payload["LayerArn"])
	}
}

func TestListLayersReturnsLatestVersion(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	doJSONRequest(t, http.MethodPost, srv.URL+"/2018-11-14/layers/shared/versions", map[string]any{"Description": "v1"}).Body.Close()
	doJSONRequest(t, http.MethodPost, srv.URL+"/2018-11-14/layers/shared/versions", map[string]any{"Description": "v2"}).Body.Close()
	doJSONRequest(t, http.MethodPost, srv.URL+"/2018-11-14/layers/data/versions", map[string]any{"Description": "first"}).Body.Close()

	resp, err := http.Get(srv.URL + "/2018-11-14/layers")
	if err != nil {
		t.Fatalf("list layers failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	payload := decodeJSONBody(t, resp)
	layers, ok := payload["Layers"].([]any)
	if !ok || len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %#v", payload["Layers"])
	}

	latestShared := map[string]any{}
	for _, entry := range layers {
		layer, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected layer object, got %#v", entry)
		}
		if layer["LayerName"] == "shared" {
			latestShared = layer
		}
	}
	if latestShared["Version"].(float64) != 2 {
		t.Fatalf("expected shared latest version 2, got %v", latestShared["Version"])
	}
}

func TestGetLayerVersion(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	doJSONRequest(t, http.MethodPost, srv.URL+"/2018-11-14/layers/image/versions", map[string]any{
		"Description":        "Image dependencies",
		"CompatibleRuntimes": []string{"python3.11"},
	}).Body.Close()

	resp, err := http.Get(srv.URL + "/2018-11-14/layers/image/versions/1")
	if err != nil {
		t.Fatalf("get layer version failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	payload := decodeJSONBody(t, resp)
	if payload["LayerName"] != "image" {
		t.Fatalf("expected layer name image, got %v", payload["LayerName"])
	}
	if payload["Description"] != "Image dependencies" {
		t.Fatalf("unexpected description: %v", payload["Description"])
	}
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

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", contentType)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return payload
}
