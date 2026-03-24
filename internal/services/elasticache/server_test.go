package elasticache

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRedisPINGReturnsPONG(t *testing.T) {
	srv := newServer()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		done <- srv.serveRedis(ln)
	}()
	defer func() {
		_ = ln.Close()
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, _ = conn.Write([]byte(respCommand("PING")))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if line != "+PONG\r\n" {
		t.Fatalf("expected +PONG, got %q", line)
	}
}

func TestRedisSetGetRoundTrip(t *testing.T) {
	srv := newServer()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		_ = srv.serveRedis(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	_, _ = conn.Write([]byte(respCommand("SET", "hello", "world")))
	setResp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SET response: %v", err)
	}
	if setResp != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", setResp)
	}

	_, _ = conn.Write([]byte(respCommand("GET", "hello")))
	bulkLen, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read GET bulk length: %v", err)
	}
	if bulkLen != "$5\r\n" {
		t.Fatalf("expected $5, got %q", bulkLen)
	}
	value, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read GET value: %v", err)
	}
	if value != "world\r\n" {
		t.Fatalf("expected world, got %q", value)
	}
}

func TestHTTPCreateAndDescribeCacheClusters(t *testing.T) {
	srv := newServer()

	createBody := performFormRequest(t, srv, map[string]string{
		"Action":         "CreateCacheCluster",
		"Version":        "2015-02-02",
		"CacheClusterId": "cluster-1",
		"Engine":         "redis",
		"NumCacheNodes":  "1",
	})
	if !strings.Contains(createBody, "<CacheClusterId>cluster-1</CacheClusterId>") {
		t.Fatalf("expected cluster id in create response, got %s", createBody)
	}

	describeBody := performFormRequest(t, srv, map[string]string{
		"Action":  "DescribeCacheClusters",
		"Version": "2015-02-02",
	})
	if !strings.Contains(describeBody, "<CacheClusterId>cluster-1</CacheClusterId>") {
		t.Fatalf("expected cluster id in describe response, got %s", describeBody)
	}
}

func performFormRequest(t *testing.T, handler http.Handler, payload map[string]string) string {
	t.Helper()
	form := url.Values{}
	for key, value := range payload {
		form.Set(key, value)
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d with body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != xmlContentType {
		t.Fatalf("expected content type %s, got %s", xmlContentType, got)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return string(body)
}

func respCommand(parts ...string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*%d\r\n", len(parts)))
	for _, part := range parts {
		b.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(part), part))
	}
	return b.String()
}
