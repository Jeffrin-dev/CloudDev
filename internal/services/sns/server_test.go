package sns

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/clouddev/clouddev/internal/services/sqs"
)

func TestCreateTopicAndListTopics(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(9324))
	t.Cleanup(srv.Close)

	resp := postForm(t, srv.URL, url.Values{"Action": {"CreateTopic"}, "Name": {"orders"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "<TopicArn>arn:aws:sns:us-east-1:000000000000:orders</TopicArn>") {
		t.Fatalf("unexpected create response: %s", body)
	}

	listResp := postForm(t, srv.URL, url.Values{"Action": {"ListTopics"}})
	listBody := readBody(t, listResp)
	if !strings.Contains(listBody, "arn:aws:sns:us-east-1:000000000000:orders") {
		t.Fatalf("expected topic in list response: %s", listBody)
	}
}

func TestSubscribePublishToSQSFlow(t *testing.T) {
	t.Parallel()

	sqsPort := freePort(t)
	go func() {
		_ = sqs.Start(sqsPort)
	}()
	waitForSQS(t, sqsPort)

	createQueueResp := httpPostForm(t, fmt.Sprintf("http://127.0.0.1:%d/", sqsPort), url.Values{
		"Action":    {"CreateQueue"},
		"QueueName": {"orders"},
	})
	createQueueBody := readBody(t, createQueueResp)
	queueURL := extractBetween(createQueueBody, "<QueueUrl>", "</QueueUrl>")
	if queueURL == "" {
		t.Fatalf("missing queue URL in response: %s", createQueueBody)
	}

	snsSrv := httptest.NewServer(newServer(sqsPort))
	t.Cleanup(snsSrv.Close)

	createTopicResp := postForm(t, snsSrv.URL, url.Values{"Action": {"CreateTopic"}, "Name": {"updates"}})
	createTopicBody := readBody(t, createTopicResp)
	topicArn := extractBetween(createTopicBody, "<TopicArn>", "</TopicArn>")
	if topicArn == "" {
		t.Fatalf("missing topic ARN in response: %s", createTopicBody)
	}

	subResp := postForm(t, snsSrv.URL, url.Values{
		"Action":   {"Subscribe"},
		"TopicArn": {topicArn},
		"Protocol": {"sqs"},
		"Endpoint": {queueURL},
	})
	subBody := readBody(t, subResp)
	if !strings.Contains(subBody, "<SubscriptionArn>") {
		t.Fatalf("missing subscription ARN: %s", subBody)
	}

	listSubsResp := postForm(t, snsSrv.URL, url.Values{"Action": {"ListSubscriptions"}})
	listSubsBody := readBody(t, listSubsResp)
	if !strings.Contains(listSubsBody, queueURL) {
		t.Fatalf("expected subscription endpoint in list response: %s", listSubsBody)
	}

	publishResp := postForm(t, snsSrv.URL, url.Values{
		"Action":   {"Publish"},
		"TopicArn": {topicArn},
		"Message":  {"hello subscribers"},
	})
	publishBody := readBody(t, publishResp)
	if !strings.Contains(publishBody, "<MessageId>msg-publish</MessageId>") {
		t.Fatalf("unexpected publish response: %s", publishBody)
	}

	receiveResp := httpPostForm(t, fmt.Sprintf("http://127.0.0.1:%d/", sqsPort), url.Values{
		"Action":   {"ReceiveMessage"},
		"QueueUrl": {queueURL},
	})
	receiveBody := readBody(t, receiveResp)
	if !strings.Contains(receiveBody, "<Body>hello subscribers</Body>") {
		t.Fatalf("expected published message in queue, got: %s", receiveBody)
	}
}

func postForm(t *testing.T, endpoint string, vals url.Values) *http.Response {
	t.Helper()
	return httpPostForm(t, endpoint, vals)
}

func httpPostForm(t *testing.T, endpoint string, vals url.Values) *http.Response {
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

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForSQS(t *testing.T, port int) {
	t.Helper()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/", port)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.PostForm(endpoint, url.Values{"Action": {"ListQueues"}})
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("sqs server on port %d did not become ready", port)
}
