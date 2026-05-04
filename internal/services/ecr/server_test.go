package ecr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateAndDescribeRepositories(t *testing.T) {
	s := newServer()

	rr := doRequest(t, s, "AmazonEC2ContainerRegistry_V20150921.CreateRepository", map[string]interface{}{"repositoryName": "app"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var createResp map[string]Repository
	if err := json.Unmarshal(rr.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	repo := createResp["repository"]
	if repo.RepositoryArn != "arn:aws:ecr:us-east-1:000000000000:repository/app" {
		t.Fatalf("unexpected repository arn: %s", repo.RepositoryArn)
	}
	if repo.RepositoryURI != "localhost:4562/app" {
		t.Fatalf("unexpected repository uri: %s", repo.RepositoryURI)
	}

	rr = doRequest(t, s, "AmazonEC2ContainerRegistry_V20150921.DescribeRepositories", map[string]interface{}{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var describeResp map[string][]Repository
	if err := json.Unmarshal(rr.Body.Bytes(), &describeResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(describeResp["repositories"]) != 1 {
		t.Fatalf("expected 1 repository got %d", len(describeResp["repositories"]))
	}
}

func TestGetAuthorizationToken(t *testing.T) {
	s := newServer()
	rr := doRequest(t, s, "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken", map[string]interface{}{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var resp struct {
		AuthorizationData []struct {
			AuthorizationToken string `json:"authorizationToken"`
			ExpiresAt          string `json:"expiresAt"`
		} `json:"authorizationData"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.AuthorizationData) != 1 {
		t.Fatalf("expected one auth token, got %d", len(resp.AuthorizationData))
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.AuthorizationData[0].AuthorizationToken)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if string(decoded) != "AWS:mock-token" {
		t.Fatalf("unexpected token payload: %s", string(decoded))
	}
	expiresAt, err := time.Parse(time.RFC3339, resp.AuthorizationData[0].ExpiresAt)
	if err != nil {
		t.Fatalf("parse expiresAt: %v", err)
	}
	if expiresAt.Before(time.Now().UTC().Add(11*time.Hour)) || expiresAt.After(time.Now().UTC().Add(13*time.Hour)) {
		t.Fatalf("unexpected expiry time: %s", expiresAt)
	}
}

func TestPutImage(t *testing.T) {
	s := newServer()
	doRequest(t, s, "AmazonEC2ContainerRegistry_V20150921.CreateRepository", map[string]interface{}{"repositoryName": "app"})

	manifest := `{"schemaVersion":2}`
	rr := doRequest(t, s, "AmazonEC2ContainerRegistry_V20150921.PutImage", map[string]interface{}{"repositoryName": "app", "imageManifest": manifest, "imageTag": "latest"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var putResp map[string]Image
	if err := json.Unmarshal(rr.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !strings.HasPrefix(putResp["image"].ImageDigest, "sha256:") {
		t.Fatalf("unexpected digest %s", putResp["image"].ImageDigest)
	}
	if putResp["image"].ImageTag != "latest" {
		t.Fatalf("unexpected image tag %s", putResp["image"].ImageTag)
	}
}

func doRequest(t *testing.T, h http.Handler, target string, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Amz-Target", target)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}
