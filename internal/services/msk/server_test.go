package msk

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateListDescribeAndBootstrapBrokers(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(newServer())
	t.Cleanup(ts.Close)

	createPayload := map[string]interface{}{
		"ClusterName":         "orders-cluster",
		"NumberOfBrokerNodes": 3,
		"KafkaVersion":        "3.7.0",
		"BrokerNodeGroupInfo": map[string]interface{}{
			"InstanceType":  "kafka.m5.large",
			"ClientSubnets": []string{"subnet-1", "subnet-2"},
		},
	}
	createResp := doRequest(t, http.MethodPost, ts.URL+"/v1/clusters", createPayload)
	defer createResp.Body.Close()

	if got := createResp.Header.Get("Content-Type"); !strings.HasPrefix(got, contentType) {
		t.Fatalf("expected content type %s, got %s", contentType, got)
	}

	var created Cluster
	decode(t, createResp, &created)
	if created.ClusterName != "orders-cluster" || created.State != "ACTIVE" {
		t.Fatalf("unexpected create response: %#v", created)
	}
	if !strings.HasPrefix(created.ClusterArn, "arn:aws:kafka:us-east-1:000000000000:cluster/orders-cluster/") {
		t.Fatalf("unexpected arn %q", created.ClusterArn)
	}

	listResp := doRequest(t, http.MethodGet, ts.URL+"/v1/clusters", nil)
	defer listResp.Body.Close()
	var listed map[string][]Cluster
	decode(t, listResp, &listed)
	if len(listed["ClusterInfoList"]) != 1 {
		t.Fatalf("expected one cluster, got %#v", listed)
	}

	describeResp := doRequest(t, http.MethodGet, ts.URL+"/v1/clusters/"+created.ClusterArn, nil)
	defer describeResp.Body.Close()
	var described map[string]Cluster
	decode(t, describeResp, &described)
	if described["ClusterInfo"].ClusterArn != created.ClusterArn {
		t.Fatalf("unexpected described cluster: %#v", described)
	}

	brokersResp := doRequest(t, http.MethodGet, ts.URL+"/v1/clusters/"+created.ClusterArn+"/bootstrap-brokers", nil)
	defer brokersResp.Body.Close()
	var brokers map[string]string
	decode(t, brokersResp, &brokers)
	if brokers["BootstrapBrokerString"] != "localhost:9092,localhost:9093" {
		t.Fatalf("unexpected bootstrap brokers: %#v", brokers)
	}
	if brokers["BootstrapBrokerStringSaslScram"] != "localhost:9094" {
		t.Fatalf("unexpected scram brokers: %#v", brokers)
	}
}

func doRequest(t *testing.T, method, url string, payload interface{}) *http.Response {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("expected 200 status, got %d", resp.StatusCode)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
