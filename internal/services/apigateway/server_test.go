package apigateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAPIAndVerifyInList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4574))
	t.Cleanup(srv.Close)

	createResp := postJSON(t, srv.URL+"/restapis", map[string]any{
		"name":        "orders-api",
		"description": "order operations",
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created map[string]any
	decodeJSON(t, createResp, &created)
	if created["id"] == "" {
		t.Fatalf("expected id in create response: %#v", created)
	}

	listResp, err := http.Get(srv.URL + "/restapis")
	if err != nil {
		t.Fatalf("list APIs: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed struct {
		Items []map[string]any `json:"item"`
	}
	decodeJSON(t, listResp, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 API, got %d", len(listed.Items))
	}
	if listed.Items[0]["name"] != "orders-api" {
		t.Fatalf("expected API name orders-api, got %#v", listed.Items[0]["name"])
	}
}

func TestCreateRouteOnAPI(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4574))
	t.Cleanup(srv.Close)

	createResp := postJSON(t, srv.URL+"/restapis", map[string]any{"name": "inventory-api"})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var created map[string]any
	decodeJSON(t, createResp, &created)
	apiID := created["id"].(string)

	routeResp := postJSON(t, srv.URL+"/restapis/"+apiID+"/resources", map[string]any{
		"path":           "/items",
		"method":         "GET",
		"lambdaFunction": "inventory-list",
	})
	if routeResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", routeResp.StatusCode)
	}

	var route map[string]any
	decodeJSON(t, routeResp, &route)
	if route["path"] != "/items" {
		t.Fatalf("expected route path /items, got %#v", route["path"])
	}
	if route["httpMethod"] != "GET" {
		t.Fatalf("expected method GET, got %#v", route["httpMethod"])
	}
	if route["lambdaFunction"] != "inventory-list" {
		t.Fatalf("expected lambdaFunction inventory-list, got %#v", route["lambdaFunction"])
	}
}

func TestDeployAPI(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4574))
	t.Cleanup(srv.Close)

	createResp := postJSON(t, srv.URL+"/restapis", map[string]any{"name": "billing-api"})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var created map[string]any
	decodeJSON(t, createResp, &created)
	apiID := created["id"].(string)

	deployResp := postJSON(t, srv.URL+"/restapis/"+apiID+"/deployments", map[string]any{"stageName": "dev"})
	if deployResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", deployResp.StatusCode)
	}

	var deployment map[string]any
	decodeJSON(t, deployResp, &deployment)
	if deployment["restApiId"] != apiID {
		t.Fatalf("expected restApiId %s, got %#v", apiID, deployment["restApiId"])
	}
	if deployment["stageName"] != "dev" {
		t.Fatalf("expected stageName dev, got %#v", deployment["stageName"])
	}
	if deployment["id"] == "" {
		t.Fatalf("expected deployment id in response: %#v", deployment)
	}
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
