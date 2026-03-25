package route53

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateHostedZone(t *testing.T) {
	srv := newServer()
	body := `<?xml version="1.0" encoding="UTF-8"?>
<CreateHostedZoneRequest>
  <Name>example.com.</Name>
  <HostedZoneConfig>
    <Comment>zone comment</Comment>
  </HostedZoneConfig>
</CreateHostedZoneRequest>`

	rec := performRequest(t, srv, http.MethodPost, "/2013-04-01/hostedzone", body, http.StatusCreated)
	assertContains(t, rec.Body.String(), "<Id>/hostedzone/Z1</Id>")
	assertContains(t, rec.Body.String(), "<Name>example.com.</Name>")
	assertContains(t, rec.Body.String(), "<Comment>zone comment</Comment>")
}

func TestChangeAndListResourceRecordSets(t *testing.T) {
	srv := newServer()
	performRequest(t, srv, http.MethodPost, "/2013-04-01/hostedzone", `
<CreateHostedZoneRequest><Name>example.com.</Name></CreateHostedZoneRequest>`, http.StatusCreated)

	changeBody := `
<ChangeResourceRecordSetsRequest>
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>api.example.com.</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
      <Change>
        <Action>UPSERT</Action>
        <ResourceRecordSet>
          <Name>api.example.com.</Name>
          <Type>A</Type>
          <TTL>120</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>5.6.7.8</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`
	performRequest(t, srv, http.MethodPost, "/2013-04-01/hostedzone/Z1/rrset", changeBody, http.StatusOK)

	listRec := performRequest(t, srv, http.MethodGet, "/2013-04-01/hostedzone/Z1/rrset", "", http.StatusOK)
	listBody := listRec.Body.String()
	assertContains(t, listBody, "<Name>api.example.com.</Name>")
	assertContains(t, listBody, "<Type>A</Type>")
	assertContains(t, listBody, "<TTL>120</TTL>")
	assertContains(t, listBody, "<Value>5.6.7.8</Value>")

	deleteBody := `
<ChangeResourceRecordSetsRequest>
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>DELETE</Action>
        <ResourceRecordSet>
          <Name>api.example.com.</Name>
          <Type>A</Type>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`
	performRequest(t, srv, http.MethodPost, "/2013-04-01/hostedzone/Z1/rrset", deleteBody, http.StatusOK)

	listRec = performRequest(t, srv, http.MethodGet, "/2013-04-01/hostedzone/Z1/rrset", "", http.StatusOK)
	if strings.Contains(listRec.Body.String(), "api.example.com.") {
		t.Fatalf("expected deleted rrset to be absent, got: %s", listRec.Body.String())
	}
}

func performRequest(t *testing.T, handler http.Handler, method, path, body string, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/xml")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != expectedStatus {
		payload, _ := io.ReadAll(rec.Body)
		t.Fatalf("expected status %d, got %d (%s)", expectedStatus, rec.Code, string(payload))
	}
	if ct := rec.Header().Get("Content-Type"); ct != xmlContentType {
		t.Fatalf("expected content type %s, got %s", xmlContentType, ct)
	}

	return rec
}

func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("expected response to contain %q, got: %s", want, body)
	}
}
