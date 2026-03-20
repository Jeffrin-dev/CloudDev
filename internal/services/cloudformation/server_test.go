package cloudformation

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateStackListStacksAndDescribeStack(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4581))
	t.Cleanup(srv.Close)

	template := `AWSTemplateFormatVersion: '2010-09-09'
Resources:
  AppBucket:
    Type: AWS::S3::Bucket
  AppQueue:
    Type: AWS::SQS::Queue`

	createResp := postForm(t, srv.URL, url.Values{
		"Action":       {"CreateStack"},
		"StackName":    {"sample-stack"},
		"TemplateBody": {template},
	})
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", createResp.StatusCode)
	}
	if got := createResp.Header.Get("Content-Type"); got != xmlContentType {
		t.Fatalf("expected content type %q, got %q", xmlContentType, got)
	}
	createBody := readBody(t, createResp.Body)
	if !strings.Contains(createBody, "<CreateStackResponse>") {
		t.Fatalf("expected CreateStackResponse, got %s", createBody)
	}
	if !strings.Contains(createBody, "sample-stack") {
		t.Fatalf("expected stack id to include stack name, got %s", createBody)
	}

	listResp := postForm(t, srv.URL, url.Values{"Action": {"ListStacks"}})
	defer listResp.Body.Close()
	listBody := readBody(t, listResp.Body)
	if !strings.Contains(listBody, "<StackName>sample-stack</StackName>") {
		t.Fatalf("expected stack in list response, got %s", listBody)
	}
	if !strings.Contains(listBody, "<StackStatus>CREATE_COMPLETE</StackStatus>") {
		t.Fatalf("expected CREATE_COMPLETE in list response, got %s", listBody)
	}

	describeResp := postForm(t, srv.URL, url.Values{
		"Action":    {"DescribeStacks"},
		"StackName": {"sample-stack"},
	})
	defer describeResp.Body.Close()
	describeBody := readBody(t, describeResp.Body)
	if !strings.Contains(describeBody, "<StackName>sample-stack</StackName>") {
		t.Fatalf("expected described stack, got %s", describeBody)
	}
	if !strings.Contains(describeBody, "AWS::S3::Bucket") || !strings.Contains(describeBody, "AWS::SQS::Queue") {
		t.Fatalf("expected original template in describe response, got %s", describeBody)
	}
}

func TestDescribeStackResourcesIncludesMappedServices(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer(4581))
	t.Cleanup(srv.Close)

	template := `{
  "Resources": {
    "Table": {"Type": "AWS::DynamoDB::Table"},
    "Handler": {"Type": "AWS::Lambda::Function"}
  }
}`

	resp := postForm(t, srv.URL, url.Values{
		"Action":       {"CreateStack"},
		"StackName":    {"json-stack"},
		"TemplateBody": {template},
	})
	resp.Body.Close()

	resourcesResp := postForm(t, srv.URL, url.Values{
		"Action":    {"DescribeStackResources"},
		"StackName": {"json-stack"},
	})
	defer resourcesResp.Body.Close()
	body := readBody(t, resourcesResp.Body)
	if !strings.Contains(body, "<Service>dynamodb</Service>") {
		t.Fatalf("expected dynamodb mapping, got %s", body)
	}
	if !strings.Contains(body, "<Service>lambda</Service>") {
		t.Fatalf("expected lambda mapping, got %s", body)
	}
	if !strings.Contains(body, "<LogicalResourceId>Handler</LogicalResourceId>") {
		t.Fatalf("expected Handler resource, got %s", body)
	}
}

func postForm(t *testing.T, target string, values url.Values) *http.Response {
	t.Helper()
	resp, err := http.PostForm(target, values)
	if err != nil {
		t.Fatalf("post form: %v", err)
	}
	return resp
}

func readBody(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}
