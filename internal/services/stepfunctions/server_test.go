package stepfunctions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateStateMachine(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	resp := doStepFunctionsRequest(t, srv.URL, "AmazonStates.CreateStateMachine", map[string]interface{}{
		"name":       "order-workflow",
		"definition": "{\"StartAt\":\"Hello\"}",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected Content-Type %q, got %q", jsonContentType, got)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	arn, _ := payload["stateMachineArn"].(string)
	if arn != "arn:aws:states:us-east-1:000000000000:stateMachine:order-workflow" {
		t.Fatalf("unexpected state machine arn: %q", arn)
	}
}

func TestStartExecution(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createResp := doStepFunctionsRequest(t, srv.URL, "AmazonStates.CreateStateMachine", map[string]interface{}{
		"name":       "payment-workflow",
		"definition": "{\"StartAt\":\"Charge\"}",
	})
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected create status 200, got %d", createResp.StatusCode)
	}
	var createPayload map[string]interface{}
	decodeBody(t, createResp.Body, &createPayload)

	stateMachineArn, _ := createPayload["stateMachineArn"].(string)
	startResp := doStepFunctionsRequest(t, srv.URL, "AmazonStates.StartExecution", map[string]interface{}{
		"stateMachineArn": stateMachineArn,
		"input":           "{\"orderId\":\"123\"}",
	})
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("expected start status 200, got %d", startResp.StatusCode)
	}
	if got := startResp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected Content-Type %q, got %q", jsonContentType, got)
	}

	var startPayload map[string]interface{}
	decodeBody(t, startResp.Body, &startPayload)
	executionArn, _ := startPayload["executionArn"].(string)
	if executionArn == "" {
		t.Fatal("expected executionArn in start response")
	}

	describeResp := doStepFunctionsRequest(t, srv.URL, "AmazonStates.DescribeExecution", map[string]interface{}{
		"executionArn": executionArn,
	})
	if describeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected describe status 200, got %d", describeResp.StatusCode)
	}
	var describePayload map[string]interface{}
	decodeBody(t, describeResp.Body, &describePayload)
	if describePayload["status"] != "SUCCEEDED" {
		t.Fatalf("expected execution status SUCCEEDED, got %#v", describePayload["status"])
	}
}

func doStepFunctionsRequest(t *testing.T, baseURL, target string, payload map[string]interface{}) *http.Response {
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

func decodeBody(t *testing.T, body io.ReadCloser, v interface{}) {
	t.Helper()
	defer body.Close()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
