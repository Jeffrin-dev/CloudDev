package dynamodbstreams

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListStreamsSupportsTableFilter(t *testing.T) {
	t.Parallel()
	srv := newServer()
	srv.publishRecord("users", StreamRecord{SizeBytes: 10, StreamViewType: "NEW_IMAGE"})
	srv.publishRecord("orders", StreamRecord{SizeBytes: 15, StreamViewType: "NEW_IMAGE"})

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.ListStreams", map[string]interface{}{})
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}
	var body map[string]interface{}
	decodeResponse(t, resp, &body)
	if len(body["Streams"].([]interface{})) != 2 {
		t.Fatalf("expected 2 streams, got %#v", body)
	}

	filtered := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.ListStreams", map[string]interface{}{"TableName": "users"})
	defer filtered.Body.Close()
	decodeResponse(t, filtered, &body)
	streams := body["Streams"].([]interface{})
	if len(streams) != 1 {
		t.Fatalf("expected 1 stream for users, got %#v", body)
	}
	entry := streams[0].(map[string]interface{})
	if entry["TableName"] != "users" {
		t.Fatalf("expected users table, got %#v", entry)
	}
}

func TestDescribeStreamHasSingleShard(t *testing.T) {
	t.Parallel()
	srv := newServer()
	srv.publishRecord("users", StreamRecord{SequenceNumber: "101", SizeBytes: 10, StreamViewType: "NEW_AND_OLD_IMAGES"})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	listResp := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.ListStreams", map[string]interface{}{})
	defer listResp.Body.Close()
	var listBody map[string]interface{}
	decodeResponse(t, listResp, &listBody)
	streamArn := listBody["Streams"].([]interface{})[0].(map[string]interface{})["StreamArn"].(string)

	descResp := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.DescribeStream", map[string]interface{}{"StreamArn": streamArn})
	defer descResp.Body.Close()
	var body map[string]interface{}
	decodeResponse(t, descResp, &body)
	desc := body["StreamDescription"].(map[string]interface{})
	if desc["StreamArn"] != streamArn {
		t.Fatalf("expected stream arn %q, got %#v", streamArn, desc)
	}
	shards := desc["Shards"].([]interface{})
	if len(shards) != 1 {
		t.Fatalf("expected one shard, got %#v", shards)
	}
	if shards[0].(map[string]interface{})["ShardId"] != defaultShardID {
		t.Fatalf("unexpected shard id: %#v", shards[0])
	}
}

func TestGetShardIteratorAndGetRecords(t *testing.T) {
	t.Parallel()
	srv := newServer()
	srv.publishRecord("users", StreamRecord{SequenceNumber: "1", SizeBytes: 10, StreamViewType: "NEW_IMAGE", NewImage: map[string]interface{}{"id": "u1"}})
	srv.publishRecord("users", StreamRecord{SequenceNumber: "2", SizeBytes: 11, StreamViewType: "NEW_IMAGE", NewImage: map[string]interface{}{"id": "u2"}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	listResp := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.ListStreams", map[string]interface{}{})
	defer listResp.Body.Close()
	var listBody map[string]interface{}
	decodeResponse(t, listResp, &listBody)
	streamArn := listBody["Streams"].([]interface{})[0].(map[string]interface{})["StreamArn"].(string)

	iterResp := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.GetShardIterator", map[string]interface{}{
		"StreamArn":         streamArn,
		"ShardId":           defaultShardID,
		"ShardIteratorType": "TRIM_HORIZON",
	})
	defer iterResp.Body.Close()
	var iterBody map[string]interface{}
	decodeResponse(t, iterResp, &iterBody)
	iterator := iterBody["ShardIterator"].(string)
	if iterator == "" {
		t.Fatal("expected shard iterator to be set")
	}

	recordsResp := doStreamsRequest(t, ts.URL, "DynamoDBStreams_20120810.GetRecords", map[string]interface{}{
		"ShardIterator": iterator,
		"Limit":         1,
	})
	defer recordsResp.Body.Close()
	var recordsBody map[string]interface{}
	decodeResponse(t, recordsResp, &recordsBody)
	records := recordsBody["Records"].([]interface{})
	if len(records) != 1 {
		t.Fatalf("expected one record due to limit, got %#v", recordsBody)
	}
	dynamodbRecord := records[0].(map[string]interface{})["dynamodb"].(map[string]interface{})
	if dynamodbRecord["SequenceNumber"] != "1" {
		t.Fatalf("expected sequence 1, got %#v", dynamodbRecord)
	}
}

func doStreamsRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
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
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	return resp
}

func decodeResponse(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
