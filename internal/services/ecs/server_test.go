package ecs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func doReq(t *testing.T, s http.Handler, target string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	req.Header.Set("X-Amz-Target", target)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestCreateAndListClusters(t *testing.T) {
	s := newServer()
	resp := doReq(t, s, targetPrefix+"CreateCluster", map[string]interface{}{"clusterName": "dev"})
	cluster := resp["cluster"].(map[string]interface{})
	if cluster["Status"] != "ACTIVE" {
		t.Fatalf("expected ACTIVE, got %v", cluster["Status"])
	}

	listed := doReq(t, s, targetPrefix+"ListClusters", map[string]interface{}{})
	arns := listed["clusterArns"].([]interface{})
	if len(arns) != 1 {
		t.Fatalf("expected 1 cluster ARN, got %d", len(arns))
	}
}

func TestRegisterTaskDefinition(t *testing.T) {
	s := newServer()
	resp := doReq(t, s, targetPrefix+"RegisterTaskDefinition", map[string]interface{}{
		"family":               "web",
		"containerDefinitions": []map[string]interface{}{{"name": "app", "image": "nginx"}},
	})
	td := resp["taskDefinition"].(map[string]interface{})
	if td["Revision"].(float64) != 1 {
		t.Fatalf("expected revision 1, got %v", td["Revision"])
	}

	list := doReq(t, s, targetPrefix+"ListTaskDefinitions", map[string]interface{}{})
	if len(list["taskDefinitionArns"].([]interface{})) != 1 {
		t.Fatalf("expected 1 task definition")
	}
}

func TestRunTask(t *testing.T) {
	s := newServer()
	doReq(t, s, targetPrefix+"CreateCluster", map[string]interface{}{"clusterName": "dev"})
	reg := doReq(t, s, targetPrefix+"RegisterTaskDefinition", map[string]interface{}{"family": "web", "containerDefinitions": []map[string]interface{}{{"name": "app"}}})
	arn := reg["taskDefinition"].(map[string]interface{})["TaskDefinitionArn"].(string)

	run := doReq(t, s, targetPrefix+"RunTask", map[string]interface{}{"cluster": "dev", "taskDefinition": arn})
	tasks := run["tasks"].([]interface{})
	if len(tasks) != 1 {
		t.Fatalf("expected one task")
	}
	task := tasks[0].(map[string]interface{})
	if task["LastStatus"] != "RUNNING" {
		t.Fatalf("expected RUNNING status, got %v", task["LastStatus"])
	}
}
