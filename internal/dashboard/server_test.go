package dashboard

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetRootReturnsHTML(t *testing.T) {
	t.Parallel()
	h := newHandler(map[string]int{"s3": 4566})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}
}

func TestGetStatusReturnsValidJSON(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	h := newHandler(map[string]int{"lambda": listener.Addr().(*net.TCPAddr).Port})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Services map[string]struct {
			Port    int  `json:"port"`
			Running bool `json:"running"`
		} `json:"services"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	svc, ok := payload.Services["lambda"]
	if !ok {
		t.Fatalf("expected lambda service in response")
	}
	if svc.Port == 0 {
		t.Fatalf("expected non-zero port in response")
	}
}

func newHandler(services map[string]int) http.Handler {
	srv := newServer(services)
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/status", srv.handleStatus)
	return mux
}
