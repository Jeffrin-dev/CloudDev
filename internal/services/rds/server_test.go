package rds

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func postForm(t *testing.T, h http.Handler, data url.Values) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != xmlContentType {
		t.Fatalf("expected content-type %q, got %q", xmlContentType, ct)
	}
	body, _ := io.ReadAll(rr.Result().Body)
	return string(body)
}

func TestCreateAndDescribeDBInstances(t *testing.T) {
	s := newServer()

	create := postForm(t, s, url.Values{
		"Action":               {"CreateDBInstance"},
		"DBInstanceIdentifier": {"db-1"},
		"DBInstanceClass":      {"db.t3.micro"},
		"Engine":               {"postgres"},
		"MasterUsername":       {"admin"},
		"DBName":               {"appdb"},
	})
	if !strings.Contains(create, "<CreateDBInstanceResponse") || !strings.Contains(create, "<Port>5432</Port>") {
		t.Fatalf("unexpected create response: %s", create)
	}

	describe := postForm(t, s, url.Values{"Action": {"DescribeDBInstances"}})
	for _, part := range []string{"<DescribeDBInstancesResponse", "<DBInstanceIdentifier>db-1</DBInstanceIdentifier>", "<DBInstanceStatus>available</DBInstanceStatus>"} {
		if !strings.Contains(describe, part) {
			t.Fatalf("describe response missing %q: %s", part, describe)
		}
	}
}

func TestStopDBInstance(t *testing.T) {
	s := newServer()
	_ = postForm(t, s, url.Values{
		"Action":               {"CreateDBInstance"},
		"DBInstanceIdentifier": {"db-2"},
		"DBInstanceClass":      {"db.t3.small"},
		"Engine":               {"mysql"},
		"MasterUsername":       {"root"},
	})

	stop := postForm(t, s, url.Values{"Action": {"StopDBInstance"}, "DBInstanceIdentifier": {"db-2"}})
	if !strings.Contains(stop, "<StopDBInstanceResponse") || !strings.Contains(stop, "<DBInstanceStatus>stopped</DBInstanceStatus>") {
		t.Fatalf("unexpected stop response: %s", stop)
	}
}

func TestCreateDBSnapshot(t *testing.T) {
	s := newServer()
	_ = postForm(t, s, url.Values{
		"Action":               {"CreateDBInstance"},
		"DBInstanceIdentifier": {"db-3"},
		"DBInstanceClass":      {"db.t3.small"},
		"Engine":               {"mysql"},
		"MasterUsername":       {"root"},
	})

	resp := postForm(t, s, url.Values{
		"Action":               {"CreateDBSnapshot"},
		"DBInstanceIdentifier": {"db-3"},
		"DBSnapshotIdentifier": {"snap-1"},
	})
	for _, part := range []string{"<CreateDBSnapshotResponse", "<DBSnapshotIdentifier>snap-1</DBSnapshotIdentifier>", "<Status>available</Status>"} {
		if !strings.Contains(resp, part) {
			t.Fatalf("snapshot response missing %q: %s", part, resp)
		}
	}
}
