package xray

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutTraceSegmentsStoresSegments(t *testing.T) {
	srv := newServer()
	segmentDoc := `{"id":"seg-1","trace_id":"trace-1","name":"orders","start_time":100.5,"end_time":101.2,"fault":false,"error":false}`

	resp := performRequest(t, srv, "/TraceSegments", map[string]any{
		"TraceSegmentDocuments": []map[string]any{{"Document": segmentDoc}},
	})

	if resp["StoredSegments"] != float64(1) {
		t.Fatalf("expected StoredSegments=1, got %v", resp["StoredSegments"])
	}
	unprocessed, ok := resp["UnprocessedTraceSegments"].([]any)
	if !ok || len(unprocessed) != 0 {
		t.Fatalf("expected no unprocessed segments, got %v", resp["UnprocessedTraceSegments"])
	}

	srv.mu.RLock()
	stored := srv.segments["trace-1"]
	srv.mu.RUnlock()
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored segment, got %d", len(stored))
	}
	if stored[0].Name != "orders" {
		t.Fatalf("expected stored service name orders, got %s", stored[0].Name)
	}
}

func TestGetTraceSummariesReturnsStoredTraces(t *testing.T) {
	srv := newServer()
	doc1 := `{"id":"seg-1","trace_id":"trace-1","name":"orders","start_time":100.0,"end_time":102.0,"fault":false,"error":false}`
	doc2 := `{"id":"seg-2","trace_id":"trace-1","name":"payments","start_time":99.0,"end_time":103.0,"fault":true,"error":false}`

	performRequest(t, srv, "/TraceSegments", map[string]any{
		"TraceSegmentDocuments": []map[string]any{{"Document": doc1}, {"Document": doc2}},
	})

	resp := performRequest(t, srv, "/TraceSummaries", map[string]any{})
	summaries := resp["TraceSummaries"].([]any)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 trace summary, got %d", len(summaries))
	}
	summary := summaries[0].(map[string]any)
	if summary["Id"] != "trace-1" {
		t.Fatalf("expected trace id trace-1, got %v", summary["Id"])
	}
	if summary["StartTime"] != 99.0 {
		t.Fatalf("expected StartTime 99.0, got %v", summary["StartTime"])
	}
	if summary["EndTime"] != 103.0 {
		t.Fatalf("expected EndTime 103.0, got %v", summary["EndTime"])
	}
	if summary["HasFault"] != true {
		t.Fatalf("expected HasFault true, got %v", summary["HasFault"])
	}
}

func performRequest(t *testing.T, handler http.Handler, path string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d with body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}

	if rec.Body.Len() == 0 {
		return map[string]any{}
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}
