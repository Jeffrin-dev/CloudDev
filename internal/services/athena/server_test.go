package athena

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartAndGetQueryExecution(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	startResp := doAthenaRequest(t, srv.URL, "AmazonAthena.StartQueryExecution", map[string]interface{}{
		"QueryString":           "SELECT 1",
		"QueryExecutionContext": map[string]interface{}{"Database": "default"},
	})
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", startResp.StatusCode)
	}
	assertContentType(t, startResp)
	var startBody map[string]string
	decodeResponse(t, startResp.Body, &startBody)
	id := startBody["QueryExecutionId"]
	if id == "" {
		t.Fatal("expected QueryExecutionId")
	}

	getResp := doAthenaRequest(t, srv.URL, "AmazonAthena.GetQueryExecution", map[string]interface{}{"QueryExecutionId": id})
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	var getBody struct {
		QueryExecution struct {
			QueryExecutionId string `json:"QueryExecutionId"`
			Query            string `json:"Query"`
			Status           struct {
				State string `json:"State"`
			} `json:"Status"`
		} `json:"QueryExecution"`
	}
	decodeResponse(t, getResp.Body, &getBody)
	if getBody.QueryExecution.QueryExecutionId != id || getBody.QueryExecution.Query != "SELECT 1" || getBody.QueryExecution.Status.State != "SUCCEEDED" {
		t.Fatalf("unexpected query execution: %#v", getBody.QueryExecution)
	}
}

func TestGetQueryResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	startResp := doAthenaRequest(t, srv.URL, "AmazonAthena.StartQueryExecution", map[string]interface{}{"QueryString": "SELECT 1"})
	var startBody map[string]string
	decodeResponse(t, startResp.Body, &startBody)

	resultsResp := doAthenaRequest(t, srv.URL, "AmazonAthena.GetQueryResults", map[string]interface{}{"QueryExecutionId": startBody["QueryExecutionId"]})
	if resultsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resultsResp.StatusCode)
	}
	var resultsBody struct {
		ResultSet struct {
			ResultSetMetadata struct {
				ColumnInfo []struct {
					Name string `json:"Name"`
				} `json:"ColumnInfo"`
			} `json:"ResultSetMetadata"`
			Rows []struct {
				Data []struct {
					VarCharValue string `json:"VarCharValue"`
				} `json:"Data"`
			} `json:"Rows"`
		} `json:"ResultSet"`
	}
	decodeResponse(t, resultsResp.Body, &resultsBody)
	if len(resultsBody.ResultSet.ResultSetMetadata.ColumnInfo) != 1 || resultsBody.ResultSet.ResultSetMetadata.ColumnInfo[0].Name != "result" {
		t.Fatalf("unexpected metadata: %#v", resultsBody.ResultSet.ResultSetMetadata)
	}
	if len(resultsBody.ResultSet.Rows) != 1 || len(resultsBody.ResultSet.Rows[0].Data) != 1 || resultsBody.ResultSet.Rows[0].Data[0].VarCharValue != "mock-result" {
		t.Fatalf("unexpected rows: %#v", resultsBody.ResultSet.Rows)
	}
}

func TestListQueryExecutions(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	doAthenaRequest(t, srv.URL, "AmazonAthena.StartQueryExecution", map[string]interface{}{"QueryString": "SELECT 1"}).Body.Close()
	doAthenaRequest(t, srv.URL, "AmazonAthena.StartQueryExecution", map[string]interface{}{"QueryString": "SELECT 2"}).Body.Close()

	listResp := doAthenaRequest(t, srv.URL, "AmazonAthena.ListQueryExecutions", map[string]interface{}{})
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listBody struct {
		QueryExecutionIds []string `json:"QueryExecutionIds"`
	}
	decodeResponse(t, listResp.Body, &listBody)
	if len(listBody.QueryExecutionIds) != 2 {
		t.Fatalf("expected 2 query execution ids, got %d", len(listBody.QueryExecutionIds))
	}
}

func doAthenaRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
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
	return resp
}

func decodeResponse(t *testing.T, r io.ReadCloser, v interface{}) {
	t.Helper()
	defer r.Close()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertContentType(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		resp.Body.Close()
		t.Fatalf("expected Content-Type %q, got %q", jsonContentType, got)
	}
}
