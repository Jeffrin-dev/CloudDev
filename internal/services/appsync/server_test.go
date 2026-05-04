package appsync

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndListGraphqlApis(t *testing.T) {
	s := newServer()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/apis", bytes.NewBufferString(`{"name":"test-api","authenticationType":"API_KEY"}`))
	createRec := httptest.NewRecorder()
	s.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var created GraphqlApi
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ApiId == "" {
		t.Fatalf("expected ApiId")
	}
	if created.Uris["GRAPHQL"] == "" {
		t.Fatalf("expected GRAPHQL URI")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/apis", nil)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var listed struct {
		GraphqlApis []GraphqlApi `json:"graphqlApis"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.GraphqlApis) != 1 {
		t.Fatalf("expected 1 API, got %d", len(listed.GraphqlApis))
	}
}

func TestCreateDataSource(t *testing.T) {
	s := newServer()
	apiID := createAPIForTests(t, s)

	req := httptest.NewRequest(http.MethodPost, "/v1/apis/"+apiID+"/datasources", bytes.NewBufferString(`{"Name":"ds1","Type":"AWS_LAMBDA","ServiceRoleArn":"arn:aws:iam::123456789012:role/appsync"}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var ds DataSource
	if err := json.NewDecoder(rec.Body).Decode(&ds); err != nil {
		t.Fatalf("decode datasource response: %v", err)
	}
	if ds.Name != "ds1" || ds.Type != "AWS_LAMBDA" {
		t.Fatalf("unexpected datasource: %+v", ds)
	}
}

func TestStartSchemaCreation(t *testing.T) {
	s := newServer()
	apiID := createAPIForTests(t, s)

	req := httptest.NewRequest(http.MethodPost, "/v1/apis/"+apiID+"/schemacreation", bytes.NewBufferString(`{"definition":"dHlwZSBRdWVyeSB7IGhlbGxvOiBTdHJpbmcgfQ=="}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode schema creation response: %v", err)
	}
	if resp["status"] != "PROCESSING" {
		t.Fatalf("expected PROCESSING, got %q", resp["status"])
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/apis/"+apiID+"/schema", nil)
	getRec := httptest.NewRecorder()
	s.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var schemaResp map[string]string
	if err := json.NewDecoder(getRec.Body).Decode(&schemaResp); err != nil {
		t.Fatalf("decode schema response: %v", err)
	}
	if schemaResp["schema"] == "" {
		t.Fatalf("expected stored schema")
	}
}

func createAPIForTests(t *testing.T, s *server) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/apis", bytes.NewBufferString(`{"name":"test-api","authenticationType":"API_KEY"}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	var api GraphqlApi
	if err := json.NewDecoder(rec.Body).Decode(&api); err != nil {
		t.Fatalf("decode create api response: %v", err)
	}
	return api.ApiId
}
