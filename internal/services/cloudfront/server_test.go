package cloudfront

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateDistribution(t *testing.T) {
	srv := newServer()
	body := `<CreateDistributionRequest>
  <DistributionConfig>
    <Comment>primary distribution</Comment>
    <Origins>
      <Items>
        <Origin><Id>origin-1</Id><DomainName>example.com</DomainName></Origin>
      </Items>
    </Origins>
  </DistributionConfig>
</CreateDistributionRequest>`

	rec := performRequest(t, srv, http.MethodPost, "/2020-05-31/distribution", body, http.StatusCreated)
	assertContains(t, rec.Body.String(), "<Id>D1</Id>")
	assertContains(t, rec.Body.String(), "<DomainName>D1.cloudfront.localhost</DomainName>")
	assertContains(t, rec.Body.String(), "<Status>Deployed</Status>")
}

func TestListDistributions(t *testing.T) {
	srv := newServer()
	performRequest(t, srv, http.MethodPost, "/2020-05-31/distribution", `<CreateDistributionRequest><DistributionConfig><Origins><Items><Origin><Id>o1</Id><DomainName>a.example.com</DomainName></Origin></Items></Origins></DistributionConfig></CreateDistributionRequest>`, http.StatusCreated)
	performRequest(t, srv, http.MethodPost, "/2020-05-31/distribution", `<CreateDistributionRequest><DistributionConfig><Comment>two</Comment><Origins><Items><Origin><Id>o2</Id><DomainName>b.example.com</DomainName></Origin></Items></Origins></DistributionConfig></CreateDistributionRequest>`, http.StatusCreated)

	rec := performRequest(t, srv, http.MethodGet, "/2020-05-31/distribution", "", http.StatusOK)
	assertContains(t, rec.Body.String(), "<Id>D1</Id>")
	assertContains(t, rec.Body.String(), "<Id>D2</Id>")
}

func TestCreateInvalidation(t *testing.T) {
	srv := newServer()
	performRequest(t, srv, http.MethodPost, "/2020-05-31/distribution", `<CreateDistributionRequest><DistributionConfig><Origins><Items><Origin><Id>o1</Id><DomainName>a.example.com</DomainName></Origin></Items></Origins></DistributionConfig></CreateDistributionRequest>`, http.StatusCreated)

	body := `<CreateInvalidationRequest>
  <InvalidationBatch>
    <Paths>
      <Items>
        <Path>/index.html</Path>
        <Path>/app.js</Path>
      </Items>
    </Paths>
  </InvalidationBatch>
</CreateInvalidationRequest>`

	rec := performRequest(t, srv, http.MethodPost, "/2020-05-31/distribution/D1/invalidation", body, http.StatusCreated)
	assertContains(t, rec.Body.String(), "<Id>I1</Id>")
	assertContains(t, rec.Body.String(), "<Status>Completed</Status>")

	listRec := performRequest(t, srv, http.MethodGet, "/2020-05-31/distribution/D1/invalidation", "", http.StatusOK)
	assertContains(t, listRec.Body.String(), "<Id>I1</Id>")
}

func performRequest(t *testing.T, handler http.Handler, method, path, body string, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
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
