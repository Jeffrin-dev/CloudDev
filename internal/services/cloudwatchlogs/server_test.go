package cloudwatchlogs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateLogGroupAndDescribeLogGroups(t *testing.T) {
	srv := newServer()

	performJSONRequest(t, srv, "Logs_20140328.CreateLogGroup", map[string]any{"logGroupName": "app-group"})
	resp := performJSONRequest(t, srv, "Logs_20140328.DescribeLogGroups", map[string]any{})

	groups := resp["logGroups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("expected 1 log group, got %d", len(groups))
	}
	group := groups[0].(map[string]any)
	if group["logGroupName"] != "app-group" {
		t.Fatalf("expected log group app-group, got %v", group["logGroupName"])
	}
}

func TestCreateLogStreamAndDescribeLogStreams(t *testing.T) {
	srv := newServer()
	performJSONRequest(t, srv, "Logs_20140328.CreateLogGroup", map[string]any{"logGroupName": "app-group"})
	performJSONRequest(t, srv, "Logs_20140328.CreateLogStream", map[string]any{"logGroupName": "app-group", "logStreamName": "stream-a"})

	resp := performJSONRequest(t, srv, "Logs_20140328.DescribeLogStreams", map[string]any{"logGroupName": "app-group"})
	streams := resp["logStreams"].([]any)
	if len(streams) != 1 {
		t.Fatalf("expected 1 log stream, got %d", len(streams))
	}
	stream := streams[0].(map[string]any)
	if stream["logStreamName"] != "stream-a" {
		t.Fatalf("expected stream-a, got %v", stream["logStreamName"])
	}
}

func TestPutLogEventsAndGetLogEvents(t *testing.T) {
	srv := newServer()
	performJSONRequest(t, srv, "Logs_20140328.CreateLogGroup", map[string]any{"logGroupName": "app-group"})
	performJSONRequest(t, srv, "Logs_20140328.CreateLogStream", map[string]any{"logGroupName": "app-group", "logStreamName": "stream-a"})

	performJSONRequest(t, srv, "Logs_20140328.PutLogEvents", map[string]any{
		"logGroupName":  "app-group",
		"logStreamName": "stream-a",
		"logEvents": []map[string]any{
			{"timestamp": 1000, "message": "first"},
			{"timestamp": 2000, "message": "second"},
		},
	})

	resp := performJSONRequest(t, srv, "Logs_20140328.GetLogEvents", map[string]any{"logGroupName": "app-group", "logStreamName": "stream-a"})
	events := resp["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].(map[string]any)["message"] != "first" {
		t.Fatalf("expected first event, got %v", events[0].(map[string]any)["message"])
	}
}

func TestFilterLogEventsByPattern(t *testing.T) {
	srv := newServer()
	performJSONRequest(t, srv, "Logs_20140328.CreateLogGroup", map[string]any{"logGroupName": "app-group"})
	performJSONRequest(t, srv, "Logs_20140328.CreateLogStream", map[string]any{"logGroupName": "app-group", "logStreamName": "stream-a"})
	performJSONRequest(t, srv, "Logs_20140328.PutLogEvents", map[string]any{
		"logGroupName":  "app-group",
		"logStreamName": "stream-a",
		"logEvents": []map[string]any{
			{"timestamp": 1000, "message": "ERROR unable to connect"},
			{"timestamp": 2000, "message": "INFO request complete"},
		},
	})

	resp := performJSONRequest(t, srv, "Logs_20140328.FilterLogEvents", map[string]any{"logGroupName": "app-group", "filterPattern": "ERROR"})
	events := resp["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(events))
	}
	if events[0].(map[string]any)["message"] != "ERROR unable to connect" {
		t.Fatalf("unexpected filtered message: %v", events[0].(map[string]any)["message"])
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
