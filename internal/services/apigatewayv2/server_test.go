package apigatewayv2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAPIAndGetAPIs(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createResp := postJSON(t, srv.URL+"/v2/apis", map[string]any{
		"Name":         "orders-http",
		"ProtocolType": "HTTP",
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	if createResp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected content type application/json, got %q", createResp.Header.Get("Content-Type"))
	}

	var created Api
	decodeJSON(t, createResp, &created)
	if created.ApiId == "" {
		t.Fatalf("expected ApiId to be set")
	}
	if created.ApiEndpoint != "http://localhost:4573/"+created.ApiId {
		t.Fatalf("unexpected endpoint %q", created.ApiEndpoint)
	}

	listResp, err := http.Get(srv.URL + "/v2/apis")
	if err != nil {
		t.Fatalf("get apis: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed struct {
		Items []Api `json:"Items"`
	}
	decodeJSON(t, listResp, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 api, got %d", len(listed.Items))
	}
	if listed.Items[0].Name != "orders-http" {
		t.Fatalf("expected api name orders-http, got %q", listed.Items[0].Name)
	}
}

func TestCreateRoute(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	api := createAPIForTest(t, srv.URL)

	routeResp := postJSON(t, srv.URL+"/v2/apis/"+api.ApiId+"/routes", map[string]any{
		"RouteKey": "GET /items",
		"Target":   "integrations/int-1",
	})
	if routeResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", routeResp.StatusCode)
	}

	var route Route
	decodeJSON(t, routeResp, &route)
	if route.RouteId == "" {
		t.Fatalf("expected RouteId to be set")
	}
	if route.RouteKey != "GET /items" {
		t.Fatalf("unexpected route key %q", route.RouteKey)
	}

	listResp, err := http.Get(srv.URL + "/v2/apis/" + api.ApiId + "/routes")
	if err != nil {
		t.Fatalf("list routes: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listed struct {
		Items []Route `json:"Items"`
	}
	decodeJSON(t, listResp, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 route, got %d", len(listed.Items))
	}
}

func TestCreateIntegration(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	api := createAPIForTest(t, srv.URL)

	integrationResp := postJSON(t, srv.URL+"/v2/apis/"+api.ApiId+"/integrations", map[string]any{
		"IntegrationType": "AWS_PROXY",
		"IntegrationUri":  "arn:aws:lambda:us-east-1:000000000000:function:orders-handler",
	})
	if integrationResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", integrationResp.StatusCode)
	}

	var integration Integration
	decodeJSON(t, integrationResp, &integration)
	if integration.IntegrationId == "" {
		t.Fatalf("expected IntegrationId to be set")
	}
	if integration.IntegrationType != "AWS_PROXY" {
		t.Fatalf("unexpected integration type %q", integration.IntegrationType)
	}

	listResp, err := http.Get(srv.URL + "/v2/apis/" + api.ApiId + "/integrations")
	if err != nil {
		t.Fatalf("list integrations: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listed struct {
		Items []Integration `json:"Items"`
	}
	decodeJSON(t, listResp, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(listed.Items))
	}
}

func createAPIForTest(t *testing.T, baseURL string) Api {
	t.Helper()
	resp := postJSON(t, baseURL+"/v2/apis", map[string]any{"Name": "test-api"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var api Api
	decodeJSON(t, resp, &api)
	return api
}

func postJSON(t *testing.T, url string, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post json: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
