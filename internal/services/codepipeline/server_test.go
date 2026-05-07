package codepipeline

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func call(t *testing.T, srv http.Handler, target string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("X-Amz-Target", target)
	res := httptest.NewRecorder()
	srv.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, jsonContentType, res.Header().Get("Content-Type"))
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	return payload
}

func TestCreatePipelineAndListPipelines(t *testing.T) {
	srv := newServer()
	create := call(t, srv, "CodePipeline_20150709.CreatePipeline", map[string]interface{}{
		"pipeline": map[string]interface{}{
			"name":    "demo",
			"roleArn": "arn:aws:iam::000000000000:role/demo",
			"stages":  []interface{}{map[string]interface{}{"name": "Source", "actions": []interface{}{map[string]interface{}{"name": "Pull", "actionTypeId": map[string]interface{}{"category": "Source"}}}}},
		},
	})
	require.Equal(t, "arn:aws:codepipeline:us-east-1:000000000000:demo", create["pipelineArn"])

	list := call(t, srv, "CodePipeline_20150709.ListPipelines", map[string]interface{}{})
	pipelines := list["pipelines"].([]interface{})
	require.Len(t, pipelines, 1)
	require.Equal(t, "demo", pipelines[0].(map[string]interface{})["name"])
}

func TestStartAndGetPipelineExecution(t *testing.T) {
	srv := newServer()
	call(t, srv, "CodePipeline_20150709.CreatePipeline", map[string]interface{}{"pipeline": map[string]interface{}{"name": "deploy", "roleArn": "role", "stages": []interface{}{}}})
	start := call(t, srv, "CodePipeline_20150709.StartPipelineExecution", map[string]interface{}{"pipelineName": "deploy"})
	execID := start["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	get := call(t, srv, "CodePipeline_20150709.GetPipelineExecution", map[string]interface{}{"pipelineName": "deploy", "pipelineExecutionId": execID})
	execution := get["pipelineExecution"].(map[string]interface{})
	require.Equal(t, "deploy", execution["pipelineName"])
	require.Equal(t, "InProgress", execution["status"])
	require.Equal(t, execID, execution["pipelineExecutionId"])
}
