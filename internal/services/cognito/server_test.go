package cognito

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUserPool(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	resp := doCognitoRequest(t, srv.URL, "AWSCognitoIdentityProviderService.CreateUserPool", map[string]any{
		"PoolName": "dev-pool",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	assertContentType(t, resp)

	var out struct {
		UserPool UserPool `json:"UserPool"`
	}
	decodeCognitoResponse(t, resp.Body, &out)

	if out.UserPool.Name != "dev-pool" {
		t.Fatalf("expected pool name dev-pool, got %q", out.UserPool.Name)
	}
	if out.UserPool.Status != "ACTIVE" {
		t.Fatalf("expected pool status ACTIVE, got %q", out.UserPool.Status)
	}
}

func TestAdminCreateUserAndListUsers(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	createPoolResp := doCognitoRequest(t, srv.URL, "AWSCognitoIdentityProviderService.CreateUserPool", map[string]any{
		"PoolName": "dev-pool",
	})
	var created struct {
		UserPool UserPool `json:"UserPool"`
	}
	decodeCognitoResponse(t, createPoolResp.Body, &created)

	createUserResp := doCognitoRequest(t, srv.URL, "AWSCognitoIdentityProviderService.AdminCreateUser", map[string]any{
		"UserPoolId": created.UserPool.Id,
		"Username":   "alice",
		"UserAttributes": []map[string]any{
			{"Name": "email", "Value": "alice@example.com"},
		},
	})
	if createUserResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", createUserResp.StatusCode)
	}
	var userOut struct {
		User User `json:"User"`
	}
	decodeCognitoResponse(t, createUserResp.Body, &userOut)
	if userOut.User.Username != "alice" {
		t.Fatalf("expected username alice, got %q", userOut.User.Username)
	}

	listUsersResp := doCognitoRequest(t, srv.URL, "AWSCognitoIdentityProviderService.ListUsers", map[string]any{
		"UserPoolId": created.UserPool.Id,
	})
	if listUsersResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listUsersResp.StatusCode)
	}
	var listed struct {
		Users []User `json:"Users"`
	}
	decodeCognitoResponse(t, listUsersResp.Body, &listed)
	if len(listed.Users) != 1 || listed.Users[0].Username != "alice" {
		t.Fatalf("unexpected users list: %#v", listed.Users)
	}
}

func TestInitiateAuth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newServer())
	t.Cleanup(srv.Close)

	resp := doCognitoRequest(t, srv.URL, "AWSCognitoIdentityProviderService.InitiateAuth", map[string]any{
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]any{
			"USERNAME": "alice",
			"PASSWORD": "password",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var out map[string]any
	decodeCognitoResponse(t, resp.Body, &out)
	auth, ok := out["AuthenticationResult"].(map[string]any)
	if !ok {
		t.Fatalf("missing authentication result: %#v", out)
	}
	if auth["IdToken"] != "mock-id-token" || auth["AccessToken"] != "mock-access-token" || auth["RefreshToken"] != "mock-refresh-token" {
		t.Fatalf("unexpected tokens: %#v", auth)
	}
	if auth["ExpiresIn"].(float64) != 3600 {
		t.Fatalf("unexpected expires in: %#v", auth["ExpiresIn"])
	}
}

func doCognitoRequest(t *testing.T, baseURL, target string, payload map[string]any) *http.Response {
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

func decodeCognitoResponse(t *testing.T, r io.ReadCloser, v any) {
	t.Helper()
	defer r.Close()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertContentType(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("Content-Type"); got != jsonContentType {
		resp.Body.Close()
		t.Fatalf("expected Content-Type %q, got %q", jsonContentType, got)
	}
}
