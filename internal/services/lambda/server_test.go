package lambda

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterFunctionAppearsInList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer("", false))
	t.Cleanup(srv.Close)

	createResp := doLambdaRequest(t, http.MethodPost, srv.URL+"/2015-03-31/functions", map[string]interface{}{
		"FunctionName": "hello",
		"Runtime":      "python3.9",
		"Handler":      "hello.handler",
		"Role":         "arn:aws:iam::000000000000:role/test",
		"Code": map[string]interface{}{
			"ZipFile": base64.StdEncoding.EncodeToString([]byte("zip-content")),
		},
	})
	if createResp.StatusCode != http.StatusCreated {
		createResp.Body.Close()
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var createBody map[string]interface{}
	decodeJSONBody(t, createResp, &createBody)
	if createBody["FunctionArn"] != "arn:aws:lambda:us-east-1:000000000000:function:hello" {
		t.Fatalf("expected function arn in create response, got %#v", createBody)
	}

	listReq, err := http.NewRequest(http.MethodGet, srv.URL+"/2015-03-31/functions", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list functions: %v", err)
	}
	defer listResp.Body.Close()

	var body map[string]interface{}
	decodeJSONBody(t, listResp, &body)
	fns, ok := body["Functions"].([]interface{})
	if !ok || len(fns) != 1 {
		t.Fatalf("expected one function, got %#v", body)
	}
	fn := fns[0].(map[string]interface{})
	if fn["FunctionName"] != "hello" {
		t.Fatalf("expected function name hello, got %#v", fn)
	}
}

func TestInvokeFunctionReturnsMockResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer("", false))
	t.Cleanup(srv.Close)

	doLambdaRequest(t, http.MethodPost, srv.URL+"/2015-03-31/functions", map[string]interface{}{"FunctionName": "echo"}).Body.Close()

	payload := map[string]interface{}{"message": "ping"}
	invokeResp := doLambdaRequest(t, http.MethodPost, srv.URL+"/2015-03-31/functions/echo/invocations", payload)
	defer invokeResp.Body.Close()
	if invokeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", invokeResp.StatusCode)
	}

	var body map[string]interface{}
	decodeJSONBody(t, invokeResp, &body)
	if body["functionName"] != "echo" {
		t.Fatalf("expected functionName echo, got %#v", body)
	}
	if body["statusCode"] != float64(200) {
		t.Fatalf("expected statusCode 200, got %#v", body)
	}
	payloadOut, ok := body["payload"].(map[string]interface{})
	if !ok || payloadOut["message"] != "ping" {
		t.Fatalf("unexpected payload %#v", body)
	}
}

func TestDeleteFunctionRemovesItFromList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer("", false))
	t.Cleanup(srv.Close)

	doLambdaRequest(t, http.MethodPost, srv.URL+"/2015-03-31/functions", map[string]interface{}{"FunctionName": "gone"}).Body.Close()

	delResp := doLambdaRequest(t, http.MethodDelete, srv.URL+"/2015-03-31/functions/gone", nil)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", delResp.StatusCode)
	}

	listReq, err := http.NewRequest(http.MethodGet, srv.URL+"/2015-03-31/functions", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list functions: %v", err)
	}
	defer listResp.Body.Close()

	var body map[string]interface{}
	decodeJSONBody(t, listResp, &body)
	fns, ok := body["Functions"].([]interface{})
	if !ok {
		t.Fatalf("expected function list, got %#v", body)
	}
	if len(fns) != 0 {
		t.Fatalf("expected no functions, got %#v", body)
	}
}

func doLambdaRequest(t *testing.T, method, url string, payload map[string]interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&buf).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", jsonContentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func decodeJSONBody(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
