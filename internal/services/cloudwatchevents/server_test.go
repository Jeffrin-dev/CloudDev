package cloudwatchevents

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPutRuleAndListRules(t *testing.T) {
	srv := newServer()

	resp := doForm(t, srv, url.Values{
		"Action":             {"PutRule"},
		"Name":               {"orders-created"},
		"ScheduleExpression": {"rate(5 minutes)"},
		"EventPattern":       {`{"source":["app.orders"]}`},
		"State":              {"ENABLED"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != xmlContentType {
		t.Fatalf("expected %s, got %s", xmlContentType, got)
	}
	if !strings.Contains(resp.Body.String(), "<RuleArn>arn:aws:events:us-east-1:000000000000:rule/orders-created</RuleArn>") {
		t.Fatalf("expected RuleArn in response, got %s", resp.Body.String())
	}

	listResp := doForm(t, srv, url.Values{
		"Action": {"ListRules"},
	})
	if !strings.Contains(listResp.Body.String(), "<Name>orders-created</Name>") {
		t.Fatalf("expected orders-created in list response, got %s", listResp.Body.String())
	}
}

func TestPutTargetsAndListTargetsByRule(t *testing.T) {
	srv := newServer()
	doForm(t, srv, url.Values{"Action": {"PutRule"}, "Name": {"orders"}})

	putTargetsResp := doForm(t, srv, url.Values{
		"Action":               {"PutTargets"},
		"Rule":                 {"orders"},
		"Targets.member.1.Id":  {"target-1"},
		"Targets.member.1.Arn": {"arn:aws:lambda:us-east-1:000000000000:function:processor"},
		"Targets.member.2.Id":  {"target-2"},
		"Targets.member.2.Arn": {"arn:aws:sqs:us-east-1:000000000000:queue/orders"},
	})
	if !strings.Contains(putTargetsResp.Body.String(), "<FailedEntryCount>0</FailedEntryCount>") {
		t.Fatalf("expected FailedEntryCount=0, got %s", putTargetsResp.Body.String())
	}

	listTargetsResp := doForm(t, srv, url.Values{
		"Action": {"ListTargetsByRule"},
		"Rule":   {"orders"},
	})
	body := listTargetsResp.Body.String()
	if !strings.Contains(body, "<Id>target-1</Id>") || !strings.Contains(body, "<Id>target-2</Id>") {
		t.Fatalf("expected both targets in response, got %s", body)
	}
}

func TestPutEvents(t *testing.T) {
	srv := newServer()

	resp := doForm(t, srv, url.Values{
		"Action":                      {"PutEvents"},
		"Entries.member.1.Source":     {"app.orders"},
		"Entries.member.1.DetailType": {"OrderCreated"},
		"Entries.member.1.Detail":     {`{"id":"o-1"}`},
		"Entries.member.2.Source":     {"app.orders"},
		"Entries.member.2.DetailType": {"OrderUpdated"},
		"Entries.member.2.Detail":     {`{"id":"o-1","status":"shipped"}`},
	})

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "<FailedEntryCount>0</FailedEntryCount>") {
		t.Fatalf("expected FailedEntryCount=0, got %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "<EventId>evt-1</EventId>") {
		t.Fatalf("expected event id in response, got %s", resp.Body.String())
	}
}

func doForm(t *testing.T, handler http.Handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
