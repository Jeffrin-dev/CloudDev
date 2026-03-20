package sts

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGetCallerIdentity(t *testing.T) {
	body := performFormRequest(t, &server{}, url.Values{"Action": {"GetCallerIdentity"}})
	assertContains(t, body, "<UserId>AIDIOSFODNN7EXAMPLE</UserId>")
	assertContains(t, body, "<Account>000000000000</Account>")
	assertContains(t, body, "<Arn>arn:aws:iam::000000000000:user/clouddev</Arn>")
}

func TestAssumeRole(t *testing.T) {
	body := performFormRequest(t, &server{}, url.Values{
		"Action":          {"AssumeRole"},
		"RoleArn":         {"arn:aws:iam::000000000000:role/demo"},
		"RoleSessionName": {"local-session"},
	})
	assertContains(t, body, "<AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId>")
	assertContains(t, body, "<SessionToken>mock-session-token</SessionToken>")
	assertContains(t, body, "<Expiration>")
}

func TestGetSessionToken(t *testing.T) {
	body := performFormRequest(t, &server{}, url.Values{"Action": {"GetSessionToken"}})
	assertContains(t, body, "<AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId>")
	assertContains(t, body, "<SessionToken>mock-session-token</SessionToken>")
	assertContains(t, body, "<GetSessionTokenResponse>")
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
	if got := rec.Header().Get("Content-Type"); got != xmlContentType {
		t.Fatalf("expected content type %s, got %s", xmlContentType, got)
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
