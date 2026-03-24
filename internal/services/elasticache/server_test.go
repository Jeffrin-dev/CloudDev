package elasticache

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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

	createResp := performJSONRequest(t, srv, "AmazonElastiCache.CreateCacheCluster", map[string]any{
		"CacheClusterId": "cluster-1",
		"Engine":         "redis",
		"EngineVersion":  "7.0",
		"NumCacheNodes":  1,
	})
	cluster := createResp["CacheCluster"].(map[string]any)
	if cluster["CacheClusterId"] != "cluster-1" {
		t.Fatalf("expected cluster-1, got %v", cluster["CacheClusterId"])
	}

	describeResp := performJSONRequest(t, srv, "AmazonElastiCache.DescribeCacheClusters", map[string]any{})
	clusters := describeResp["CacheClusters"].([]any)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	described := clusters[0].(map[string]any)
	if described["CacheClusterId"] != "cluster-1" {
		t.Fatalf("expected cluster-1, got %v", described["CacheClusterId"])
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

func respCommand(parts ...string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*%d\r\n", len(parts)))
	for _, part := range parts {
		b.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(part), part))
	}
	return b.String()
}
