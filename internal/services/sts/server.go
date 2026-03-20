package sts

import (
	"fmt"
	"net/http"
	"time"
)

const xmlContentType = "text/xml"

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), &server{})
}

type server struct{}

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
	case "GetCallerIdentity":
		s.getCallerIdentity(w)
	case "AssumeRole":
		s.assumeRole(w, r)
	case "GetSessionToken":
		s.getSessionToken(w)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) getCallerIdentity(w http.ResponseWriter) {
	writeXML(w, "<GetCallerIdentityResponse><GetCallerIdentityResult><UserId>AIDIOSFODNN7EXAMPLE</UserId><Account>000000000000</Account><Arn>arn:aws:iam::000000000000:user/clouddev</Arn></GetCallerIdentityResult><ResponseMetadata><RequestId>req-getcalleridentity</RequestId></ResponseMetadata></GetCallerIdentityResponse>")
}

func (s *server) assumeRole(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("RoleArn") == "" || r.FormValue("RoleSessionName") == "" {
		writeError(w, "MissingParameter", "RoleArn and RoleSessionName are required")
		return
	}

	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	writeXML(w, fmt.Sprintf("<AssumeRoleResponse><AssumeRoleResult><Credentials><AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId><SecretAccessKey>wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY</SecretAccessKey><SessionToken>mock-session-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult><ResponseMetadata><RequestId>req-assumerole</RequestId></ResponseMetadata></AssumeRoleResponse>", expiration))
}

func (s *server) getSessionToken(w http.ResponseWriter) {
	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	writeXML(w, fmt.Sprintf("<GetSessionTokenResponse><GetSessionTokenResult><Credentials><AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId><SecretAccessKey>wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY</SecretAccessKey><SessionToken>mock-session-token</SessionToken><Expiration>%s</Expiration></Credentials></GetSessionTokenResult><ResponseMetadata><RequestId>req-getsessiontoken</RequestId></ResponseMetadata></GetSessionTokenResponse>", expiration))
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", code, message))
}
