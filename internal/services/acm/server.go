package acm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const jsonContentType = "application/x-amz-json-1.1"

type Certificate struct {
	CertificateArn           string   `json:"CertificateArn"`
	DomainName               string   `json:"DomainName"`
	Status                   string   `json:"Status"`
	Type                     string   `json:"Type"`
	SubjectAlternativeNames  []string `json:"SubjectAlternativeNames"`
	IssuedAt                 string   `json:"IssuedAt"`
}

type server struct {
	mu           sync.RWMutex
	certificates map[string]Certificate
	tags         map[string][]map[string]string
}

func newServer() *server {
	return &server{certificates: make(map[string]Certificate), tags: make(map[string][]map[string]string)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"__type": "MethodNotAllowedException", "message": "Only POST is supported"})
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"__type": "SerializationException", "message": "Invalid JSON body"})
		return
	}

	switch r.Header.Get("X-Amz-Target") {
	case "CertificateManager.RequestCertificate":
		s.requestCertificate(w, payload)
	case "CertificateManager.DescribeCertificate":
		s.describeCertificate(w, payload)
	case "CertificateManager.ListCertificates":
		s.listCertificates(w)
	case "CertificateManager.DeleteCertificate":
		s.deleteCertificate(w, payload)
	case "CertificateManager.GetCertificate":
		s.getCertificate(w, payload)
	case "CertificateManager.AddTagsToCertificate":
		s.addTagsToCertificate(w, payload)
	case "CertificateManager.ListTagsForCertificate":
		s.listTagsForCertificate(w, payload)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"__type": "UnknownOperationException", "message": "Unknown X-Amz-Target"})
	}
}

func (s *server) requestCertificate(w http.ResponseWriter, payload map[string]any) {
	domainName, _ := payload["DomainName"].(string)
	sans := toStringSlice(payload["SubjectAlternativeNames"])
	arn := fmt.Sprintf("arn:aws:acm:us-east-1:000000000000:certificate/%s", mustUUID())
	cert := Certificate{CertificateArn: arn, DomainName: domainName, Status: "ISSUED", Type: "AMAZON_ISSUED", SubjectAlternativeNames: sans, IssuedAt: time.Now().UTC().Format(time.RFC3339)}

	s.mu.Lock()
	s.certificates[arn] = cert
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"CertificateArn": arn})
}

func (s *server) describeCertificate(w http.ResponseWriter, payload map[string]any) {
	arn, _ := payload["CertificateArn"].(string)
	s.mu.RLock()
	cert, ok := s.certificates[arn]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"__type": "ResourceNotFoundException", "message": "Certificate not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]Certificate{"Certificate": cert})
}

func (s *server) listCertificates(w http.ResponseWriter) {
	s.mu.RLock()
	list := make([]Certificate, 0, len(s.certificates))
	for _, cert := range s.certificates {
		list = append(list, cert)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"CertificateSummaryList": list})
}

func (s *server) deleteCertificate(w http.ResponseWriter, payload map[string]any) {
	arn, _ := payload["CertificateArn"].(string)
	s.mu.Lock()
	delete(s.certificates, arn)
	delete(s.tags, arn)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) getCertificate(w http.ResponseWriter, payload map[string]any) {
	arn, _ := payload["CertificateArn"].(string)
	s.mu.RLock()
	_, ok := s.certificates[arn]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"__type": "ResourceNotFoundException", "message": "Certificate not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"Certificate":       "-----BEGIN CERTIFICATE-----\nMIID...MOCKCERT...\n-----END CERTIFICATE-----",
		"CertificateChain":  "-----BEGIN CERTIFICATE-----\nMIID...MOCKCHAIN...\n-----END CERTIFICATE-----",
	})
}

func (s *server) addTagsToCertificate(w http.ResponseWriter, payload map[string]any) {
	arn, _ := payload["CertificateArn"].(string)
	tags := toTags(payload["Tags"])
	s.mu.Lock()
	s.tags[arn] = append(s.tags[arn], tags...)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) listTagsForCertificate(w http.ResponseWriter, payload map[string]any) {
	arn, _ := payload["CertificateArn"].(string)
	s.mu.RLock()
	tags := append([]map[string]string(nil), s.tags[arn]...)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"Tags": tags})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toTags(v any) []map[string]string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]string, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag := map[string]string{}
		if key, ok := m["Key"].(string); ok {
			tag["Key"] = key
		}
		if value, ok := m["Value"].(string); ok {
			tag["Value"] = value
		}
		out = append(out, tag)
	}
	return out
}

func mustUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexStr := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
}
