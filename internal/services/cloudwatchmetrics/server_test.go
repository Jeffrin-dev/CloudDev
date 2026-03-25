package cloudwatchmetrics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutMetricDataAndListMetrics(t *testing.T) {
	srv := newServer()

	performJSONRequest(t, srv, "GraniteServiceVersion20100801.PutMetricData", map[string]any{
		"Namespace": "CloudDev/App",
		"MetricData": []map[string]any{
			{"MetricName": "Latency", "Value": 10.0, "Unit": "Milliseconds"},
			{"MetricName": "Latency", "Value": 20.0, "Unit": "Milliseconds"},
		},
	})

	resp := performJSONRequest(t, srv, "GraniteServiceVersion20100801.ListMetrics", map[string]any{})
	metrics := resp["Metrics"].([]any)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	metric := metrics[0].(map[string]any)
	if metric["MetricName"] != "Latency" {
		t.Fatalf("expected metric Latency, got %v", metric["MetricName"])
	}
	if metric["Namespace"] != "CloudDev/App" {
		t.Fatalf("expected namespace CloudDev/App, got %v", metric["Namespace"])
	}
}

func TestGetMetricStatistics(t *testing.T) {
	srv := newServer()

	performJSONRequest(t, srv, "GraniteServiceVersion20100801.PutMetricData", map[string]any{
		"Namespace": "CloudDev/App",
		"MetricData": []map[string]any{
			{"MetricName": "RequestCount", "Value": 2.0},
			{"MetricName": "RequestCount", "Value": 6.0},
			{"MetricName": "RequestCount", "Value": 4.0},
		},
	})

	resp := performJSONRequest(t, srv, "GraniteServiceVersion20100801.GetMetricStatistics", map[string]any{
		"Namespace":  "CloudDev/App",
		"MetricName": "RequestCount",
	})

	datapoints := resp["Datapoints"].([]any)
	if len(datapoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %d", len(datapoints))
	}
	stats := datapoints[0].(map[string]any)
	if stats["SampleCount"] != 3.0 {
		t.Fatalf("expected sample count 3, got %v", stats["SampleCount"])
	}
	if stats["Sum"] != 12.0 {
		t.Fatalf("expected sum 12, got %v", stats["Sum"])
	}
	if stats["Average"] != 4.0 {
		t.Fatalf("expected average 4, got %v", stats["Average"])
	}
	if stats["Minimum"] != 2.0 {
		t.Fatalf("expected min 2, got %v", stats["Minimum"])
	}
	if stats["Maximum"] != 6.0 {
		t.Fatalf("expected max 6, got %v", stats["Maximum"])
	}
}

func TestPutMetricAlarmAndDescribeAlarms(t *testing.T) {
	srv := newServer()

	performJSONRequest(t, srv, "GraniteServiceVersion20100801.PutMetricAlarm", map[string]any{
		"AlarmName":          "HighLatency",
		"MetricName":         "Latency",
		"Namespace":          "CloudDev/App",
		"Threshold":          100.0,
		"ComparisonOperator": "GreaterThanThreshold",
	})

	resp := performJSONRequest(t, srv, "GraniteServiceVersion20100801.DescribeAlarms", map[string]any{})
	alarms := resp["MetricAlarms"].([]any)
	if len(alarms) != 1 {
		t.Fatalf("expected 1 alarm, got %d", len(alarms))
	}
	alarm := alarms[0].(map[string]any)
	if alarm["AlarmName"] != "HighLatency" {
		t.Fatalf("expected alarm HighLatency, got %v", alarm["AlarmName"])
	}
	if alarm["StateValue"] != "OK" {
		t.Fatalf("expected alarm state OK, got %v", alarm["StateValue"])
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
