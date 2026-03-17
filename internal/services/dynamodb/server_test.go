package dynamodb

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestCreateTableAndListTables(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createPayload := map[string]interface{}{
		"TableName": "users",
		"KeySchema": []map[string]interface{}{{"AttributeName": "id", "KeyType": "HASH"}},
	}
	resp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.CreateTable", createPayload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	listResp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.ListTables", map[string]interface{}{})
	defer listResp.Body.Close()
	if got := listResp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %q, got %q", jsonContentType, got)
	}
	var body map[string]interface{}
	decodeBody(t, listResp.Body, &body)
	tableNamesRaw, ok := body["TableNames"].([]interface{})
	if !ok || len(tableNamesRaw) != 1 || tableNamesRaw[0] != "users" {
		t.Fatalf("unexpected list tables response: %#v", body)
	}
}

func TestPutItemAndGetItem(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createTable(t, srv.URL, "users", "id")

	item := map[string]interface{}{
		"id":   map[string]interface{}{"S": "u1"},
		"name": map[string]interface{}{"S": "Alice"},
	}
	putResp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.PutItem", map[string]interface{}{
		"TableName": "users",
		"Item":      item,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", putResp.StatusCode)
	}

	getResp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.GetItem", map[string]interface{}{
		"TableName": "users",
		"Key": map[string]interface{}{
			"id": map[string]interface{}{"S": "u1"},
		},
	})
	defer getResp.Body.Close()
	var body map[string]interface{}
	decodeBody(t, getResp.Body, &body)
	itemOut, ok := body["Item"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected item in response, got %#v", body)
	}
	nameAttr, ok := itemOut["name"].(map[string]interface{})
	if !ok || nameAttr["S"] != "Alice" {
		t.Fatalf("unexpected item payload: %#v", itemOut)
	}
}

func TestDeleteItemRemovesData(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createTable(t, srv.URL, "users", "id")
	doDynamoRequest(t, srv.URL, "DynamoDB_20120810.PutItem", map[string]interface{}{
		"TableName": "users",
		"Item": map[string]interface{}{
			"id": map[string]interface{}{"S": "u2"},
		},
	}).Body.Close()

	delResp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.DeleteItem", map[string]interface{}{
		"TableName": "users",
		"Key": map[string]interface{}{
			"id": map[string]interface{}{"S": "u2"},
		},
	})
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", delResp.StatusCode)
	}

	getResp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.GetItem", map[string]interface{}{
		"TableName": "users",
		"Key": map[string]interface{}{
			"id": map[string]interface{}{"S": "u2"},
		},
	})
	defer getResp.Body.Close()
	var body map[string]interface{}
	decodeBody(t, getResp.Body, &body)
	if _, ok := body["Item"]; ok {
		t.Fatalf("expected item to be gone, got %#v", body)
	}
}

func TestScanReturnsAllItems(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createTable(t, srv.URL, "users", "id")
	for _, id := range []string{"a", "b", "c"} {
		doDynamoRequest(t, srv.URL, "DynamoDB_20120810.PutItem", map[string]interface{}{
			"TableName": "users",
			"Item": map[string]interface{}{
				"id": map[string]interface{}{"S": id},
			},
		}).Body.Close()
	}

	scanResp := doDynamoRequest(t, srv.URL, "DynamoDB_20120810.Scan", map[string]interface{}{"TableName": "users"})
	defer scanResp.Body.Close()
	var body map[string]interface{}
	decodeBody(t, scanResp.Body, &body)
	if body["Count"] != float64(3) {
		t.Fatalf("expected count 3, got %#v", body)
	}
	itemsRaw, ok := body["Items"].([]interface{})
	if !ok || len(itemsRaw) != 3 {
		t.Fatalf("expected 3 items, got %#v", body)
	}

	ids := make([]string, 0, 3)
	for _, entry := range itemsRaw {
		item := entry.(map[string]interface{})
		attr := item["id"].(map[string]interface{})
		ids = append(ids, attr["S"].(string))
	}
	sort.Strings(ids)
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func createTable(t *testing.T, baseURL, name, hashKey string) {
	t.Helper()
	resp := doDynamoRequest(t, baseURL, "DynamoDB_20120810.CreateTable", map[string]interface{}{
		"TableName": name,
		"KeySchema": []map[string]interface{}{{"AttributeName": hashKey, "KeyType": "HASH"}},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create table failed: %d", resp.StatusCode)
	}
}

func doDynamoRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("Content-Type", jsonContentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func decodeBody(t *testing.T, r io.Reader, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
