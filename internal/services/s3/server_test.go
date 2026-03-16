package s3

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() http.Handler {
	return &server{buckets: make(map[string]*bucket)}
}

func TestCreateBucketAndList(t *testing.T) {
	h := newTestServer()

	req := httptest.NewRequest(http.MethodPut, "/test-bucket", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Fatalf("expected application/xml, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "<Name>test-bucket</Name>") {
		t.Fatalf("expected bucket in list response, got %s", w.Body.String())
	}
}

func TestPutAndGetObject(t *testing.T) {
	h := newTestServer()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/test-bucket", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 creating bucket, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/test-bucket/file.txt", strings.NewReader("hello clouddev")))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 putting object, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test-bucket/file.txt", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 getting object, got %d", w.Code)
	}
	if got := w.Body.String(); got != "hello clouddev" {
		t.Fatalf("expected object content, got %q", got)
	}
}

func TestDeleteObjectThenGetReturns404(t *testing.T) {
	h := newTestServer()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/test-bucket", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 creating bucket, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/test-bucket/file.txt", strings.NewReader("delete me")))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 putting object, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/test-bucket/file.txt", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 deleting object, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test-bucket/file.txt", nil))
	if w.Code != http.StatusNotFound {
		body, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("expected 404 getting deleted object, got %d body=%s", w.Code, string(body))
	}
}
