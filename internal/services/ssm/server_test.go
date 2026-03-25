package ssm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutAndGetParameter(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	putResp := doSSMRequest(t, srv.URL, "AmazonSSM.PutParameter", map[string]interface{}{
		"Name":  "/app/config/key",
		"Value": "v1",
		"Type":  "String",
	})
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected put status 200, got %d", putResp.StatusCode)
	}
	assertContentType(t, putResp)
	var putBody map[string]interface{}
	decodeResponse(t, putResp.Body, &putBody)
	if putBody["Version"] != float64(1) {
		t.Fatalf("expected version 1, got %#v", putBody)
	}

	overwriteResp := doSSMRequest(t, srv.URL, "AmazonSSM.PutParameter", map[string]interface{}{
		"Name":  "/app/config/key",
		"Value": "v2",
		"Type":  "String",
	})
	if overwriteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected overwrite status 200, got %d", overwriteResp.StatusCode)
	}
	var overwriteBody map[string]interface{}
	decodeResponse(t, overwriteResp.Body, &overwriteBody)
	if overwriteBody["Version"] != float64(2) {
		t.Fatalf("expected version 2 after overwrite, got %#v", overwriteBody)
	}

	getResp := doSSMRequest(t, srv.URL, "AmazonSSM.GetParameter", map[string]interface{}{"Name": "/app/config/key"})
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected get status 200, got %d", getResp.StatusCode)
	}
	var getBody struct {
		Parameter Parameter `json:"Parameter"`
	}
	decodeResponse(t, getResp.Body, &getBody)
	if getBody.Parameter.Value != "v2" || getBody.Parameter.Version != 2 {
		t.Fatalf("unexpected parameter response: %#v", getBody.Parameter)
	}
}

func TestGetParametersByPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	doSSMRequest(t, srv.URL, "AmazonSSM.PutParameter", map[string]interface{}{"Name": "/app/db/user", "Value": "u", "Type": "String"}).Body.Close()
	doSSMRequest(t, srv.URL, "AmazonSSM.PutParameter", map[string]interface{}{"Name": "/app/db/pass", "Value": "p", "Type": "SecureString"}).Body.Close()
	doSSMRequest(t, srv.URL, "AmazonSSM.PutParameter", map[string]interface{}{"Name": "/other/key", "Value": "x", "Type": "String"}).Body.Close()

	resp := doSSMRequest(t, srv.URL, "AmazonSSM.GetParametersByPath", map[string]interface{}{"Path": "/app/db"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get-by-path status 200, got %d", resp.StatusCode)
	}
	var body struct {
		Parameters []Parameter `json:"Parameters"`
	}
	decodeResponse(t, resp.Body, &body)
	if len(body.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(body.Parameters))
	}
	if body.Parameters[0].Name != "/app/db/pass" || body.Parameters[1].Name != "/app/db/user" {
		t.Fatalf("unexpected parameter order/content: %#v", body.Parameters)
	}
}

func TestDeleteParameter(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	doSSMRequest(t, srv.URL, "AmazonSSM.PutParameter", map[string]interface{}{"Name": "/delete/me", "Value": "1", "Type": "String"}).Body.Close()

	deleteResp := doSSMRequest(t, srv.URL, "AmazonSSM.DeleteParameter", map[string]interface{}{"Name": "/delete/me"})
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d", deleteResp.StatusCode)
	}
	deleteResp.Body.Close()

	getResp := doSSMRequest(t, srv.URL, "AmazonSSM.GetParameter", map[string]interface{}{"Name": "/delete/me"})
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing parameter status 400, got %d", getResp.StatusCode)
	}
	var errResp map[string]interface{}
	if err := json.NewDecoder(getResp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["__type"] != "ParameterNotFound" {
		t.Fatalf("expected ParameterNotFound, got %#v", errResp)
	}
}

func doSSMRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
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
