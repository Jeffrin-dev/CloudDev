package eventbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateEventBus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(0, 0))
	t.Cleanup(srv.Close)

	resp := doRequest(t, srv.URL, "AmazonEventBridge.CreateEventBus", map[string]any{"Name": "orders"})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected Content-Type %q, got %q", jsonContentType, got)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	arn, _ := payload["EventBusArn"].(string)
	if arn != "arn:aws:events:us-east-1:000000000000:event-bus/orders" {
		t.Fatalf("unexpected EventBusArn: %q", arn)
	}
}

func TestPutRule(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(0, 0))
	t.Cleanup(srv.Close)

	createResp := doRequest(t, srv.URL, "AmazonEventBridge.CreateEventBus", map[string]any{"Name": "orders"})
	createResp.Body.Close()

	resp := doRequest(t, srv.URL, "AmazonEventBridge.PutRule", map[string]any{
		"Name":         "order-created",
		"EventBusName": "orders",
		"EventPattern": `{"source":["app.orders"],"detail":{"status":["created"]}}`,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	listResp := doRequest(t, srv.URL, "AmazonEventBridge.ListRules", map[string]any{"EventBusName": "orders"})
	defer listResp.Body.Close()
	var payload struct {
		Rules []Rule `json:"Rules"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(payload.Rules) != 1 || payload.Rules[0].Name != "order-created" {
		t.Fatalf("expected rule order-created, got %#v", payload.Rules)
	}
}

func TestPutEventsForwardsToLambdaAndSQS(t *testing.T) {
	t.Parallel()

	lambdaCalls := make(chan map[string]any, 1)
	lambdaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		lambdaCalls <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(lambdaSrv.Close)

	sqsCalls := make(chan string, 1)
	sqsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			sqsCalls <- r.FormValue("MessageBody")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<ok/>"))
	}))
	t.Cleanup(sqsSrv.Close)

	lambdaPort := mustPort(t, lambdaSrv.Listener.Addr().String())
	sqsPort := mustPort(t, sqsSrv.Listener.Addr().String())

	srv := httptest.NewServer(newServer(lambdaPort, sqsPort))
	t.Cleanup(srv.Close)

	doRequest(t, srv.URL, "AmazonEventBridge.PutRule", map[string]any{
		"Name":         "order-created",
		"EventBusName": "default",
		"EventPattern": `{"source":["app.orders"],"detail-type":["OrderCreated"],"detail":{"status":["created"]}}`,
	}).Body.Close()

	doRequest(t, srv.URL, "AmazonEventBridge.PutTargets", map[string]any{
		"Rule": "order-created",
		"Targets": []map[string]any{
			{"Id": "lambda-1", "Arn": "arn:aws:lambda:us-east-1:000000000000:function:processor"},
			{"Id": "sqs-1", "Arn": "arn:aws:sqs:us-east-1:000000000000:orders-queue"},
		},
	}).Body.Close()

	resp := doRequest(t, srv.URL, "AmazonEventBridge.PutEvents", map[string]any{
		"Entries": []map[string]any{
			{
				"Source":       "app.orders",
				"DetailType":   "OrderCreated",
				"Detail":       `{"status":"created","id":"o-1"}`,
				"EventBusName": "default",
			},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	select {
	case payload := <-lambdaCalls:
		if payload["source"] != "app.orders" {
			t.Fatalf("unexpected lambda payload: %#v", payload)
		}
	default:
		t.Fatal("expected lambda invocation")
	}

	select {
	case body := <-sqsCalls:
		if body == "" {
			t.Fatal("expected sqs message body")
		}
	default:
		t.Fatal("expected sqs message")
	}
}

func doRequest(t *testing.T, baseURL, target string, payload map[string]any) *http.Response {
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

func mustPort(t *testing.T, hostPort string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return p
}
