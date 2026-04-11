package ses

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSendEmail(t *testing.T) {
	s := newServer()

	body := performFormRequest(t, s, url.Values{
		"Action":                           {"SendEmail"},
		"Source":                           {"sender@example.com"},
		"Destination.ToAddresses.member.1": {"alice@example.com"},
		"Destination.ToAddresses.member.2": {"bob@example.com"},
		"Message.Subject.Data":             {"Hello"},
		"Message.Body.Text.Data":           {"Hi team"},
	})

	assertContains(t, body, "<SendEmailResponse>")
	assertContains(t, body, "<MessageId>msg-1</MessageId>")

	if len(s.emails) != 1 {
		t.Fatalf("expected 1 stored email, got %d", len(s.emails))
	}
	if s.emails[0].From != "sender@example.com" {
		t.Fatalf("unexpected from: %s", s.emails[0].From)
	}
	if got := strings.Join(s.emails[0].To, ","); got != "alice@example.com,bob@example.com" {
		t.Fatalf("unexpected to addresses: %s", got)
	}
	if s.emails[0].Subject != "Hello" || s.emails[0].Body != "Hi team" {
		t.Fatalf("unexpected message content: %+v", s.emails[0])
	}
}

func TestVerifyEmailIdentityAndListIdentities(t *testing.T) {
	s := newServer()

	performFormRequest(t, s, url.Values{
		"Action":       {"VerifyEmailIdentity"},
		"EmailAddress": {"verified@example.com"},
	})

	body := performFormRequest(t, s, url.Values{"Action": {"ListIdentities"}})
	assertContains(t, body, "<ListIdentitiesResponse>")
	assertContains(t, body, "<member>verified@example.com</member>")
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
