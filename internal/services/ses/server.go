package ses

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const xmlContentType = "text/xml"

type Email struct {
	MessageId string
	From      string
	To        []string
	Subject   string
	Body      string
	Timestamp time.Time
}

type server struct {
	mu         sync.Mutex
	emails     []Email
	identities map[string]struct{}
	nextID     int64
}

func newServer() *server {
	return &server{
		emails:     make([]Email, 0),
		identities: make(map[string]struct{}),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "InvalidAction", "Only POST is supported")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, "InvalidParameterValue", "Could not parse form body")
		return
	}

	switch r.FormValue("Action") {
	case "SendEmail":
		s.sendEmail(w, r)
	case "SendRawEmail":
		s.sendRawEmail(w, r)
	case "ListIdentities":
		s.listIdentities(w)
	case "VerifyEmailIdentity":
		s.verifyEmailIdentity(w, r)
	case "DeleteIdentity":
		s.deleteIdentity(w, r)
	case "GetSendStatistics":
		s.getSendStatistics(w)
	case "GetSendQuota":
		s.getSendQuota(w)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) sendEmail(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("Source")
	to := destinationToAddresses(r)
	subject := r.FormValue("Message.Subject.Data")
	body := r.FormValue("Message.Body.Text.Data")

	s.mu.Lock()
	email := Email{
		MessageId: s.nextMessageIDLocked(),
		From:      source,
		To:        to,
		Subject:   subject,
		Body:      body,
		Timestamp: time.Now().UTC(),
	}
	s.emails = append(s.emails, email)
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<SendEmailResponse><SendEmailResult><MessageId>%s</MessageId></SendEmailResult><ResponseMetadata><RequestId>req-sendemail</RequestId></ResponseMetadata></SendEmailResponse>", email.MessageId))
}

func (s *server) sendRawEmail(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("Source")
	rawData := r.FormValue("RawMessage.Data")

	s.mu.Lock()
	email := Email{
		MessageId: s.nextMessageIDLocked(),
		From:      source,
		To:        destinationToAddresses(r),
		Body:      rawData,
		Timestamp: time.Now().UTC(),
	}
	s.emails = append(s.emails, email)
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<SendRawEmailResponse><SendRawEmailResult><MessageId>%s</MessageId></SendRawEmailResult><ResponseMetadata><RequestId>req-sendrawemail</RequestId></ResponseMetadata></SendRawEmailResponse>", email.MessageId))
}

func (s *server) listIdentities(w http.ResponseWriter) {
	s.mu.Lock()
	identities := make([]string, 0, len(s.identities))
	for identity := range s.identities {
		identities = append(identities, identity)
	}
	s.mu.Unlock()
	sort.Strings(identities)

	members := ""
	for _, identity := range identities {
		members += fmt.Sprintf("<member>%s</member>", identity)
	}

	writeXML(w, fmt.Sprintf("<ListIdentitiesResponse><ListIdentitiesResult><Identities>%s</Identities></ListIdentitiesResult><ResponseMetadata><RequestId>req-listidentities</RequestId></ResponseMetadata></ListIdentitiesResponse>", members))
}

func (s *server) verifyEmailIdentity(w http.ResponseWriter, r *http.Request) {
	emailAddress := r.FormValue("EmailAddress")
	if emailAddress == "" {
		writeError(w, "MissingParameter", "EmailAddress is required")
		return
	}

	s.mu.Lock()
	s.identities[emailAddress] = struct{}{}
	s.mu.Unlock()

	writeXML(w, "<VerifyEmailIdentityResponse><ResponseMetadata><RequestId>req-verifyemailidentity</RequestId></ResponseMetadata></VerifyEmailIdentityResponse>")
}

func (s *server) deleteIdentity(w http.ResponseWriter, r *http.Request) {
	identity := r.FormValue("Identity")
	if identity == "" {
		writeError(w, "MissingParameter", "Identity is required")
		return
	}

	s.mu.Lock()
	delete(s.identities, identity)
	s.mu.Unlock()

	writeXML(w, "<DeleteIdentityResponse><ResponseMetadata><RequestId>req-deleteidentity</RequestId></ResponseMetadata></DeleteIdentityResponse>")
}

func (s *server) getSendStatistics(w http.ResponseWriter) {
	s.mu.Lock()
	deliveryAttempts := len(s.emails)
	s.mu.Unlock()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	writeXML(w, fmt.Sprintf("<GetSendStatisticsResponse><GetSendStatisticsResult><SendDataPoints><member><Timestamp>%s</Timestamp><DeliveryAttempts>%d</DeliveryAttempts><Bounces>0</Bounces><Complaints>0</Complaints><Rejects>0</Rejects></member></SendDataPoints></GetSendStatisticsResult><ResponseMetadata><RequestId>req-getsendstatistics</RequestId></ResponseMetadata></GetSendStatisticsResponse>", timestamp, deliveryAttempts))
}

func (s *server) getSendQuota(w http.ResponseWriter) {
	writeXML(w, "<GetSendQuotaResponse><GetSendQuotaResult><Max24HourSend>50000</Max24HourSend><SentLast24Hours>0</SentLast24Hours><MaxSendRate>14</MaxSendRate></GetSendQuotaResult><ResponseMetadata><RequestId>req-getsendquota</RequestId></ResponseMetadata></GetSendQuotaResponse>")
}

func (s *server) nextMessageIDLocked() string {
	s.nextID++
	return fmt.Sprintf("msg-%d", s.nextID)
}

func destinationToAddresses(r *http.Request) []string {
	type member struct {
		idx   int
		value string
	}

	members := make([]member, 0)
	for k, vals := range r.Form {
		if !strings.HasPrefix(k, "Destination.ToAddresses.member.") {
			continue
		}
		idxStr := strings.TrimPrefix(k, "Destination.ToAddresses.member.")
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			continue
		}
		for _, v := range vals {
			members = append(members, member{idx: idx, value: v})
		}
	}

	sort.Slice(members, func(i, j int) bool { return members[i].idx < members[j].idx })

	to := make([]string, 0, len(members))
	for _, m := range members {
		to = append(to, m.value)
	}
	return to
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", code, message))
}
