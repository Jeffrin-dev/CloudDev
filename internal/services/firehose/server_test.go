package firehose

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndListDeliveryStreams(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	resp := doFirehoseRequest(t, ts.URL, "Firehose_20150804.CreateDeliveryStream", map[string]interface{}{
		"DeliveryStreamName": "events",
		"DeliveryStreamType": "DirectPut",
	})
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}

	listResp := doFirehoseRequest(t, ts.URL, "Firehose_20150804.ListDeliveryStreams", map[string]interface{}{})
	defer listResp.Body.Close()
	var listBody map[string]interface{}
	decodeResponse(t, listResp, &listBody)

	names := listBody["DeliveryStreamNames"].([]interface{})
	if len(names) != 1 || names[0] != "events" {
		t.Fatalf("expected one delivery stream named events, got %#v", listBody)
	}
}

func TestPutRecord(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	doFirehoseRequest(t, ts.URL, "Firehose_20150804.CreateDeliveryStream", map[string]interface{}{
		"DeliveryStreamName": "logs",
		"DeliveryStreamType": "DirectPut",
	}).Body.Close()

	resp := doFirehoseRequest(t, ts.URL, "Firehose_20150804.PutRecord", map[string]interface{}{
		"DeliveryStreamName": "logs",
		"Record": map[string]interface{}{
			"Data": "aGVsbG8=",
		},
	})
	defer resp.Body.Close()
	var body map[string]interface{}
	decodeResponse(t, resp, &body)
	if body["RecordId"] == "" {
		t.Fatalf("expected RecordId in response, got %#v", body)
	}
}

func TestPutRecordBatch(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	doFirehoseRequest(t, ts.URL, "Firehose_20150804.CreateDeliveryStream", map[string]interface{}{
		"DeliveryStreamName": "batch-stream",
		"DeliveryStreamType": "DirectPut",
	}).Body.Close()

	resp := doFirehoseRequest(t, ts.URL, "Firehose_20150804.PutRecordBatch", map[string]interface{}{
		"DeliveryStreamName": "batch-stream",
		"Records": []map[string]interface{}{
			{"Data": "Zmlyc3Q="},
			{"Data": "c2Vjb25k"},
		},
	})
	defer resp.Body.Close()

	var body map[string]interface{}
	decodeResponse(t, resp, &body)
	if body["FailedPutCount"].(float64) != 0 {
		t.Fatalf("expected FailedPutCount=0, got %#v", body)
	}
	responses := body["RequestResponses"].([]interface{})
	if len(responses) != 2 {
		t.Fatalf("expected 2 request responses, got %#v", body)
	}
	for _, item := range responses {
		entry := item.(map[string]interface{})
		if entry["RecordId"] == "" {
			t.Fatalf("expected RecordId for all responses, got %#v", body)
		}
	}
}

func doFirehoseRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("Content-Type", jsonContentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	return resp
}

func decodeResponse(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
