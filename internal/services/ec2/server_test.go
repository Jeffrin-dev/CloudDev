package ec2

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func postForm(t *testing.T, s http.Handler, values url.Values) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != xmlContentType {
		t.Fatalf("expected content type %s got %s", xmlContentType, got)
	}
	return w.Body.String()
}

func TestRunAndDescribeInstances(t *testing.T) {
	s := newServer()
	run := postForm(t, s, url.Values{"Action": {"RunInstances"}, "ImageId": {"ami-test"}, "InstanceType": {"t3.small"}, "MinCount": {"1"}, "MaxCount": {"1"}, "KeyName": {"dev-key"}})
	if !strings.Contains(run, "<RunInstancesResponse") || !strings.Contains(run, "<instanceState><name>running</name></instanceState>") {
		t.Fatalf("unexpected run response: %s", run)
	}
	re := regexp.MustCompile(`<instanceId>(i-[0-9a-f]{8})</instanceId>`)
	match := re.FindStringSubmatch(run)
	if len(match) != 2 {
		t.Fatalf("expected generated instance id in response: %s", run)
	}
	if !strings.Contains(run, "<privateIpAddress>10.0.0.1</privateIpAddress>") || !strings.Contains(run, "<ipAddress>54.0.0.1</ipAddress>") {
		t.Fatalf("expected assigned ip addresses: %s", run)
	}
	desc := postForm(t, s, url.Values{"Action": {"DescribeInstances"}})
	if !strings.Contains(desc, match[1]) {
		t.Fatalf("describe response missing instance %s: %s", match[1], desc)
	}
}

func TestTerminateInstances(t *testing.T) {
	s := newServer()
	run := postForm(t, s, url.Values{"Action": {"RunInstances"}})
	re := regexp.MustCompile(`<instanceId>(i-[0-9a-f]{8})</instanceId>`)
	id := re.FindStringSubmatch(run)[1]
	_ = postForm(t, s, url.Values{"Action": {"TerminateInstances"}, "InstanceId": {id}})
	desc := postForm(t, s, url.Values{"Action": {"DescribeInstances"}})
	if !strings.Contains(desc, "<instanceState><name>terminated</name></instanceState>") {
		t.Fatalf("expected terminated state, got: %s", desc)
	}
}

func TestCreateKeyPair(t *testing.T) {
	s := newServer()
	resp := postForm(t, s, url.Values{"Action": {"CreateKeyPair"}, "KeyName": {"my-key"}})
	if !strings.Contains(resp, "<keyName>my-key</keyName>") || !strings.Contains(resp, "aa:bb:cc:dd") || !strings.Contains(resp, "BEGIN PRIVATE KEY") {
		t.Fatalf("unexpected create key pair response: %s", resp)
	}
	list := postForm(t, s, url.Values{"Action": {"DescribeKeyPairs"}})
	if !strings.Contains(list, "<keyName>my-key</keyName>") {
		t.Fatalf("expected key in describe response: %s", list)
	}
}
