package opensearch

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateListDescribeDomain(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	createBody := map[string]string{
		"DomainName":    "movies",
		"EngineVersion": "OpenSearch_2.11",
	}
	resp := doJSON(t, http.MethodPost, ts.URL+"/2021-01-01/opensearch/domain", createBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	var createResp struct {
		DomainStatus Domain `json:"DomainStatus"`
	}
	decodeJSON(t, resp, &createResp)
	if createResp.DomainStatus.DomainId != "000000000000/movies" {
		t.Fatalf("unexpected domain id: %s", createResp.DomainStatus.DomainId)
	}
	if createResp.DomainStatus.ARN != "arn:aws:es:us-east-1:000000000000:domain/movies" {
		t.Fatalf("unexpected arn: %s", createResp.DomainStatus.ARN)
	}
	if createResp.DomainStatus.Endpoint != "movies.us-east-1.es.localhost" {
		t.Fatalf("unexpected endpoint: %s", createResp.DomainStatus.Endpoint)
	}

	listResp := doJSON(t, http.MethodGet, ts.URL+"/2021-01-01/opensearch/domain", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listResp.StatusCode)
	}
	var listBody struct {
		DomainNames []struct {
			DomainName string `json:"DomainName"`
		} `json:"DomainNames"`
	}
	decodeJSON(t, listResp, &listBody)
	if len(listBody.DomainNames) != 1 || listBody.DomainNames[0].DomainName != "movies" {
		t.Fatalf("unexpected list response: %+v", listBody.DomainNames)
	}

	describeResp := doJSON(t, http.MethodGet, ts.URL+"/2021-01-01/opensearch/domain/movies", nil)
	if describeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", describeResp.StatusCode)
	}
	var describeBody struct {
		DomainStatus Domain `json:"DomainStatus"`
	}
	decodeJSON(t, describeResp, &describeBody)
	if !describeBody.DomainStatus.Created {
		t.Fatalf("expected Created=true")
	}
	if describeBody.DomainStatus.EngineVersion != "OpenSearch_2.11" {
		t.Fatalf("unexpected engine version: %s", describeBody.DomainStatus.EngineVersion)
	}
}

func TestAddAndListTags(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	arn := "arn:aws:es:us-east-1:000000000000:domain/movies"
	addBody := map[string]interface{}{
		"ARN": arn,
		"TagList": []map[string]string{
			{"Key": "env", "Value": "dev"},
		},
	}
	resp := doJSON(t, http.MethodPost, ts.URL+"/2021-01-01/opensearch/domain/movies/tags", addBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	listResp := doJSON(t, http.MethodGet, ts.URL+"/2021-01-01/opensearch/domain/movies/tags?arn="+arn, nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listResp.StatusCode)
	}
	var listBody struct {
		TagList []Tag `json:"TagList"`
	}
	decodeJSON(t, listResp, &listBody)
	if len(listBody.TagList) != 1 || listBody.TagList[0].Key != "env" || listBody.TagList[0].Value != "dev" {
		t.Fatalf("unexpected tags: %+v", listBody.TagList)
	}
}

func doJSON(t *testing.T, method, url string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, out interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
