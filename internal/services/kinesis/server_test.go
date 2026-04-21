package kinesis

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateStreamAndListStreams(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	resp := doKinesisRequest(t, ts.URL, "Kinesis_20131202.CreateStream", map[string]interface{}{
		"StreamName": "orders",
		"ShardCount": 2,
	})
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}

	listResp := doKinesisRequest(t, ts.URL, "Kinesis_20131202.ListStreams", map[string]interface{}{})
	defer listResp.Body.Close()
	var listBody map[string]interface{}
	decodeResponse(t, listResp, &listBody)

	names := listBody["StreamNames"].([]interface{})
	if len(names) != 1 || names[0] != "orders" {
		t.Fatalf("expected one stream named orders, got %#v", listBody)
	}
}

func TestPutRecordAndGetRecords(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	createResp := doKinesisRequest(t, ts.URL, "Kinesis_20131202.CreateStream", map[string]interface{}{
		"StreamName": "events",
		"ShardCount": 1,
	})
	createResp.Body.Close()

	putResp := doKinesisRequest(t, ts.URL, "Kinesis_20131202.PutRecord", map[string]interface{}{
		"StreamName":   "events",
		"Data":         "aGVsbG8=",
		"PartitionKey": "pk-1",
	})
	defer putResp.Body.Close()
	var putBody map[string]interface{}
	decodeResponse(t, putResp, &putBody)

	if putBody["SequenceNumber"] == "" {
		t.Fatalf("expected sequence number, got %#v", putBody)
	}
	shardID := putBody["ShardId"].(string)

	iterResp := doKinesisRequest(t, ts.URL, "Kinesis_20131202.GetShardIterator", map[string]interface{}{
		"StreamName":        "events",
		"ShardId":           shardID,
		"ShardIteratorType": "TRIM_HORIZON",
	})
	defer iterResp.Body.Close()
	var iterBody map[string]interface{}
	decodeResponse(t, iterResp, &iterBody)
	iterator := iterBody["ShardIterator"].(string)
	if iterator == "" {
		t.Fatal("expected shard iterator")
	}

	recordsResp := doKinesisRequest(t, ts.URL, "Kinesis_20131202.GetRecords", map[string]interface{}{
		"ShardIterator": iterator,
		"Limit":         10,
	})
	defer recordsResp.Body.Close()
	var recordsBody map[string]interface{}
	decodeResponse(t, recordsResp, &recordsBody)
	records := recordsBody["Records"].([]interface{})
	if len(records) != 1 {
		t.Fatalf("expected one record, got %#v", recordsBody)
	}
	rec := records[0].(map[string]interface{})
	if rec["Data"] != "aGVsbG8=" {
		t.Fatalf("unexpected record data: %#v", rec)
	}
	if recordsBody["NextShardIterator"].(string) == iterator {
		t.Fatalf("expected next iterator to advance, got %#v", recordsBody)
	}
}

func doKinesisRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(body))
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
