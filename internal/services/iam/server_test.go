package iam

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateUser(t *testing.T) {
	srv := newServer()
	body := performFormRequest(t, srv, url.Values{"Action": {"CreateUser"}, "UserName": {"alice"}})
	assertContains(t, body, "<UserName>alice</UserName>")
	assertContains(t, body, "arn:aws:iam::000000000000:user/alice")
}

func TestCreateRole(t *testing.T) {
	srv := newServer()
	body := performFormRequest(t, srv, url.Values{
		"Action":                   {"CreateRole"},
		"RoleName":                 {"app-role"},
		"AssumeRolePolicyDocument": {"{\"Version\":\"2012-10-17\"}"},
	})
	assertContains(t, body, "<RoleName>app-role</RoleName>")
	assertContains(t, body, "arn:aws:iam::000000000000:role/app-role")
}

func TestAssumeRole(t *testing.T) {
	srv := newServer()
	performFormRequest(t, srv, url.Values{
		"Action":                   {"CreateRole"},
		"RoleName":                 {"app-role"},
		"AssumeRolePolicyDocument": {"{\"Version\":\"2012-10-17\"}"},
	})

	body := performFormRequest(t, srv, url.Values{
		"Action":          {"AssumeRole"},
		"RoleArn":         {"arn:aws:iam::000000000000:role/app-role"},
		"RoleSessionName": {"dev-session"},
	})
	assertContains(t, body, "<AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId>")
	assertContains(t, body, "<SessionToken>mock-session-token</SessionToken>")
	assertContains(t, body, "<AssumedRoleId>")
}

func performFormRequest(t *testing.T, handler http.Handler, form url.Values) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != xmlContentType {
		t.Fatalf("expected content type %s, got %s", xmlContentType, rec.Header().Get("Content-Type"))
	}
	data, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("expected response to contain %q, got %s", want, body)
	}
}
