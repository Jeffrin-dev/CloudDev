package s3

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateBucketAppearsInList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/test-bucket", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create bucket failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	listResp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("list buckets failed: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	if got := listResp.Header.Get("Content-Type"); got != xmlContentType {
		t.Fatalf("expected Content-Type %q, got %q", xmlContentType, got)
	}

	body, _ := io.ReadAll(listResp.Body)
	if !strings.Contains(string(body), "<Name>test-bucket</Name>") {
		t.Fatalf("expected bucket in list response, got: %s", string(body))
	}
}

func TestPutObjectAndRetrieve(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	_, _ = http.DefaultClient.Do(mustRequest(t, http.MethodPut, srv.URL+"/test-bucket", nil))

	putReq := mustRequest(t, http.MethodPut, srv.URL+"/test-bucket/file.txt", strings.NewReader("hello"))
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("put object failed: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", putResp.StatusCode)
	}

	getResp, err := http.Get(srv.URL + "/test-bucket/file.txt")
	if err != nil {
		t.Fatalf("get object failed: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	body, _ := io.ReadAll(getResp.Body)
	if string(body) != "hello" {
		t.Fatalf("expected hello, got %q", string(body))
	}
}

func TestDeleteObjectThenGetReturnsNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	_, _ = http.DefaultClient.Do(mustRequest(t, http.MethodPut, srv.URL+"/test-bucket", nil))
	_, _ = http.DefaultClient.Do(mustRequest(t, http.MethodPut, srv.URL+"/test-bucket/file.txt", strings.NewReader("bye")))

	delResp, err := http.DefaultClient.Do(mustRequest(t, http.MethodDelete, srv.URL+"/test-bucket/file.txt", nil))
	if err != nil {
		t.Fatalf("delete object failed: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	getResp, err := http.Get(srv.URL + "/test-bucket/file.txt")
	if err != nil {
		t.Fatalf("get object failed: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", getResp.StatusCode)
	}
}

func mustRequest(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}
