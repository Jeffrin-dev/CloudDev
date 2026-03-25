package sqs

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateFIFOQueueSetsAttribute(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9330))
	t.Cleanup(srv.Close)

	createResp := postForm(t, srv.URL, url.Values{"Action": {"CreateQueue"}, "QueueName": {"orders.fifo"}})
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", createResp.StatusCode)
	}
	_ = readBody(t, createResp)

	attrResp := postForm(t, srv.URL, url.Values{"Action": {"GetQueueAttributes"}, "QueueUrl": {"http://localhost:9330/queue/orders.fifo"}})
	if attrResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", attrResp.StatusCode)
	}
	attrXML := readBody(t, attrResp)
	if !strings.Contains(attrXML, "<Name>FifoQueue</Name><Value>true</Value>") {
		t.Fatalf("expected fifo attribute in get attributes response: %s", attrXML)
	}
}

func TestFIFOQueueSendAndReceiveInOrder(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9331))
	t.Cleanup(srv.Close)

	postForm(t, srv.URL, url.Values{"Action": {"CreateQueue"}, "QueueName": {"jobs.fifo"}})

	postForm(t, srv.URL, url.Values{
		"Action":                 {"SendMessage"},
		"QueueUrl":               {"http://localhost:9331/queue/jobs.fifo"},
		"MessageBody":            {"first"},
		"MessageGroupId":         {"group-1"},
		"MessageDeduplicationId": {"dedup-1"},
	})
	postForm(t, srv.URL, url.Values{
		"Action":                 {"SendMessage"},
		"QueueUrl":               {"http://localhost:9331/queue/jobs.fifo"},
		"MessageBody":            {"second"},
		"MessageGroupId":         {"group-1"},
		"MessageDeduplicationId": {"dedup-2"},
	})

	firstResp := postForm(t, srv.URL, url.Values{"Action": {"ReceiveMessage"}, "QueueUrl": {"http://localhost:9331/queue/jobs.fifo"}})
	firstXML := readBody(t, firstResp)
	if !strings.Contains(firstXML, "<Body>first</Body>") {
		t.Fatalf("expected first message, got: %s", firstXML)
	}

	receipt := extractBetween(firstXML, "<ReceiptHandle>", "</ReceiptHandle>")
	if receipt == "" {
		t.Fatalf("missing receipt handle in first receive: %s", firstXML)
	}
	_ = readBody(t, postForm(t, srv.URL, url.Values{"Action": {"DeleteMessage"}, "QueueUrl": {"http://localhost:9331/queue/jobs.fifo"}, "ReceiptHandle": {receipt}}))

	secondResp := postForm(t, srv.URL, url.Values{"Action": {"ReceiveMessage"}, "QueueUrl": {"http://localhost:9331/queue/jobs.fifo"}})
	secondXML := readBody(t, secondResp)
	if !strings.Contains(secondXML, "<Body>second</Body>") {
		t.Fatalf("expected second message, got: %s", secondXML)
	}
}

func TestFIFODeduplicationIgnoresDuplicateWithinWindow(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9332))
	t.Cleanup(srv.Close)

	postForm(t, srv.URL, url.Values{"Action": {"CreateQueue"}, "QueueName": {"events.fifo"}})

	firstSend := postForm(t, srv.URL, url.Values{
		"Action":                 {"SendMessage"},
		"QueueUrl":               {"http://localhost:9332/queue/events.fifo"},
		"MessageBody":            {"only-once"},
		"MessageGroupId":         {"group-1"},
		"MessageDeduplicationId": {"dup-1"},
	})
	if firstSend.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", firstSend.StatusCode)
	}
	_ = readBody(t, firstSend)

	duplicateSend := postForm(t, srv.URL, url.Values{
		"Action":                 {"SendMessage"},
		"QueueUrl":               {"http://localhost:9332/queue/events.fifo"},
		"MessageBody":            {"only-once"},
		"MessageGroupId":         {"group-1"},
		"MessageDeduplicationId": {"dup-1"},
	})
	if duplicateSend.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", duplicateSend.StatusCode)
	}
	_ = readBody(t, duplicateSend)

	receiveResp := postForm(t, srv.URL, url.Values{"Action": {"ReceiveMessage"}, "QueueUrl": {"http://localhost:9332/queue/events.fifo"}})
	receiveXML := readBody(t, receiveResp)
	if !strings.Contains(receiveXML, "<Body>only-once</Body>") {
		t.Fatalf("expected first message present, got: %s", receiveXML)
	}

	receipt := extractBetween(receiveXML, "<ReceiptHandle>", "</ReceiptHandle>")
	if receipt == "" {
		t.Fatalf("missing receipt handle in receive response: %s", receiveXML)
	}
	_ = readBody(t, postForm(t, srv.URL, url.Values{"Action": {"DeleteMessage"}, "QueueUrl": {"http://localhost:9332/queue/events.fifo"}, "ReceiptHandle": {receipt}}))

	receiveAgainResp := postForm(t, srv.URL, url.Values{"Action": {"ReceiveMessage"}, "QueueUrl": {"http://localhost:9332/queue/events.fifo"}})
	receiveAgainXML := readBody(t, receiveAgainResp)
	if strings.Contains(receiveAgainXML, "<Body>only-once</Body>") {
		t.Fatalf("deduplicated message should not have been enqueued twice: %s", receiveAgainXML)
	}
}
