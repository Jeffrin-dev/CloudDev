package firehose

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"

type DeliveryStream struct {
	DeliveryStreamName   string
	DeliveryStreamARN    string
	DeliveryStreamStatus string
	DeliveryStreamType   string
}

type FirehoseRecord struct {
	RecordId string
	Data     string
}

type server struct {
	mu      sync.RWMutex
	streams map[string]DeliveryStream
	records map[string][]FirehoseRecord
}

func newServer() *server {
	return &server{streams: make(map[string]DeliveryStream), records: make(map[string][]FirehoseRecord)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "MissingAction", "X-Amz-Target is required")
		return
	}

	payload := map[string]interface{}{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "SerializationException", "Invalid JSON body")
		return
	}

	switch target {
	case "Firehose_20150804.CreateDeliveryStream":
		s.handleCreateDeliveryStream(w, payload)
	case "Firehose_20150804.DeleteDeliveryStream":
		s.handleDeleteDeliveryStream(w, payload)
	case "Firehose_20150804.ListDeliveryStreams":
		s.handleListDeliveryStreams(w)
	case "Firehose_20150804.DescribeDeliveryStream":
		s.handleDescribeDeliveryStream(w, payload)
	case "Firehose_20150804.PutRecord":
		s.handlePutRecord(w, payload)
	case "Firehose_20150804.PutRecordBatch":
		s.handlePutRecordBatch(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateDeliveryStream(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "DeliveryStreamName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "DeliveryStreamName is required")
		return
	}
	streamType, ok := stringField(payload, "DeliveryStreamType")
	if !ok || streamType == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "DeliveryStreamType is required")
		return
	}

	stream := DeliveryStream{
		DeliveryStreamName:   name,
		DeliveryStreamARN:    fmt.Sprintf("arn:aws:firehose:us-east-1:000000000000:deliverystream/%s", name),
		DeliveryStreamStatus: "ACTIVE",
		DeliveryStreamType:   streamType,
	}

	s.mu.Lock()
	s.streams[name] = stream
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleDeleteDeliveryStream(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "DeliveryStreamName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "DeliveryStreamName is required")
		return
	}
	s.mu.Lock()
	delete(s.streams, name)
	delete(s.records, name)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleListDeliveryStreams(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]string, 0, len(s.streams))
	for name := range s.streams {
		names = append(names, name)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	writeJSON(w, http.StatusOK, map[string]interface{}{"DeliveryStreamNames": names, "HasMoreDeliveryStreams": false})
}

func (s *server) handleDescribeDeliveryStream(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "DeliveryStreamName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "DeliveryStreamName is required")
		return
	}
	s.mu.RLock()
	stream, exists := s.streams[name]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Delivery stream does not exist")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"DeliveryStreamDescription": stream})
}

func (s *server) handlePutRecord(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "DeliveryStreamName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "DeliveryStreamName is required")
		return
	}
	recordPayload, ok := payload["Record"].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Record is required")
		return
	}
	data, ok := stringField(recordPayload, "Data")
	if !ok || data == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Record.Data is required")
		return
	}

	s.mu.Lock()
	if _, exists := s.streams[name]; !exists {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Delivery stream does not exist")
		return
	}
	rec := FirehoseRecord{RecordId: newRecordID(), Data: data}
	s.records[name] = append(s.records[name], rec)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"RecordId": rec.RecordId})
}

func (s *server) handlePutRecordBatch(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "DeliveryStreamName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "DeliveryStreamName is required")
		return
	}
	records, ok := payload["Records"].([]interface{})
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Records is required")
		return
	}

	s.mu.Lock()
	if _, exists := s.streams[name]; !exists {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Delivery stream does not exist")
		return
	}
	responses := make([]map[string]string, 0, len(records))
	for _, item := range records {
		recMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		data, _ := stringField(recMap, "Data")
		rec := FirehoseRecord{RecordId: newRecordID(), Data: data}
		s.records[name] = append(s.records[name], rec)
		responses = append(responses, map[string]string{"RecordId": rec.RecordId})
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"FailedPutCount": 0, "RequestResponses": responses})
}

func stringField(payload map[string]interface{}, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]interface{}{"__type": code, "message": msg})
}

func newRecordID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
