package sqs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateQueueAndListQueues(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9324))
	t.Cleanup(srv.Close)

	createBody := url.Values{}
	createBody.Set("Action", "CreateQueue")
	createBody.Set("QueueName", "orders")

	createResp := postForm(t, srv.URL, createBody)
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", createResp.StatusCode)
	}
	createXML := readBody(t, createResp)
	if !strings.Contains(createXML, "<QueueUrl>http://localhost:9324/queue/orders</QueueUrl>") {
		t.Fatalf("unexpected create response: %s", createXML)
	}

	listBody := url.Values{}
	listBody.Set("Action", "ListQueues")
	listResp := postForm(t, srv.URL, listBody)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	listXML := readBody(t, listResp)
	if !strings.Contains(listXML, "<QueueUrl>http://localhost:9324/queue/orders</QueueUrl>") {
		t.Fatalf("expected queue in list response: %s", listXML)
	}
}

func TestSendAndReceiveMessage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9325))
	t.Cleanup(srv.Close)

	postForm(t, srv.URL, url.Values{"Action": {"CreateQueue"}, "QueueName": {"jobs"}})

	send := url.Values{}
	send.Set("Action", "SendMessage")
	send.Set("QueueUrl", "http://localhost:9325/queue/jobs")
	send.Set("MessageBody", "hello world")
	sendResp := postForm(t, srv.URL, send)
	if sendResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", sendResp.StatusCode)
	}

	receive := url.Values{}
	receive.Set("Action", "ReceiveMessage")
	receive.Set("QueueUrl", "http://localhost:9325/queue/jobs")
	recvResp := postForm(t, srv.URL, receive)
	if recvResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", recvResp.StatusCode)
	}
	recvXML := readBody(t, recvResp)
	if !strings.Contains(recvXML, "<Body>hello world</Body>") {
		t.Fatalf("expected message body in receive response: %s", recvXML)
	}
}

func TestDeleteMessageRemovesMessageFromQueue(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9326))
	t.Cleanup(srv.Close)

	postForm(t, srv.URL, url.Values{"Action": {"CreateQueue"}, "QueueName": {"events"}})
	postForm(t, srv.URL, url.Values{"Action": {"SendMessage"}, "QueueUrl": {"http://localhost:9326/queue/events"}, "MessageBody": {"remove-me"}})

	receiveResp := postForm(t, srv.URL, url.Values{"Action": {"ReceiveMessage"}, "QueueUrl": {"http://localhost:9326/queue/events"}})
	receiveXML := readBody(t, receiveResp)
	receipt := extractBetween(receiveXML, "<ReceiptHandle>", "</ReceiptHandle>")
	if receipt == "" {
		t.Fatalf("missing receipt handle in receive response: %s", receiveXML)
	}

	delResp := postForm(t, srv.URL, url.Values{"Action": {"DeleteMessage"}, "QueueUrl": {"http://localhost:9326/queue/events"}, "ReceiptHandle": {receipt}})
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", delResp.StatusCode)
	}
	_ = readBody(t, delResp)

	receiveAgain := postForm(t, srv.URL, url.Values{"Action": {"ReceiveMessage"}, "QueueUrl": {"http://localhost:9326/queue/events"}})
	receiveAgainXML := readBody(t, receiveAgain)
	if strings.Contains(receiveAgainXML, "<Body>remove-me</Body>") {
		t.Fatalf("message still present after delete: %s", receiveAgainXML)
	}
}

func postForm(t *testing.T, endpoint string, vals url.Values) *http.Response {
	t.Helper()
	resp, err := http.PostForm(endpoint, vals)
	if err != nil {
		t.Fatalf("post form: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func extractBetween(s, start, end string) string {
	startIdx := strings.Index(s, start)
	if startIdx < 0 {
		return ""
	}
	startIdx += len(start)
	endIdx := strings.Index(s[startIdx:], end)
	if endIdx < 0 {
		return ""
	}
	return s[startIdx : startIdx+endIdx]
}
