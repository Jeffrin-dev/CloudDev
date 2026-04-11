package dynamodbstreams

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const jsonContentType = "application/x-amz-json-1.0"

type Stream struct {
	StreamArn    string
	TableName    string
	StreamLabel  string
	StreamStatus string
}

type StreamRecord struct {
	SequenceNumber string
	SizeBytes      int
	StreamViewType string
	NewImage       map[string]interface{}
	OldImage       map[string]interface{}
}

type iteratorState struct {
	streamArn string
	index     int
}

type streamState struct {
	stream  Stream
	records []StreamRecord
}

type server struct {
	mu           sync.RWMutex
	streams      map[string]*streamState
	tableToArn   map[string]string
	iterators    map[string]iteratorState
	nextIterator int
	nextSequence int64
}

var activeServer *server

func newServer() *server {
	return &server{
		streams:    make(map[string]*streamState),
		tableToArn: make(map[string]string),
		iterators:  make(map[string]iteratorState),
	}
}

func Start(port int) error {
	srv := newServer()
	activeServer = srv
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func PublishRecord(tableName string, record StreamRecord) {
	if activeServer == nil {
		return
	}
	activeServer.publishRecord(tableName, record)
}

func (s *server) publishRecord(tableName string, record StreamRecord) {
	if tableName == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	arn, ok := s.tableToArn[tableName]
	if !ok {
		label := time.Now().UTC().Format("2006-01-02T15:04:05.000")
		arn = fmt.Sprintf("arn:aws:dynamodb:local:000000000000:table/%s/stream/%s", tableName, label)
		s.tableToArn[tableName] = arn
		s.streams[arn] = &streamState{
			stream: Stream{
				StreamArn:    arn,
				TableName:    tableName,
				StreamLabel:  label,
				StreamStatus: "ENABLED",
			},
		}
	}

	if record.SequenceNumber == "" {
		s.nextSequence++
		record.SequenceNumber = strconv.FormatInt(s.nextSequence, 10)
	}

	state := s.streams[arn]
	state.records = append(state.records, record)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Missing X-Amz-Target")
		return
	}

	payload := map[string]interface{}{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "SerializationException", "Invalid JSON body")
		return
	}

	switch target {
	case "DynamoDBStreams_20120810.ListStreams":
		s.handleListStreams(w, payload)
	case "DynamoDBStreams_20120810.DescribeStream":
		s.handleDescribeStream(w, payload)
	case "DynamoDBStreams_20120810.GetShardIterator":
		s.handleGetShardIterator(w, payload)
	case "DynamoDBStreams_20120810.GetRecords":
		s.handleGetRecords(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleListStreams(w http.ResponseWriter, payload map[string]interface{}) {
	tableFilter, _ := stringField(payload, "TableName")

	s.mu.RLock()
	arns := make([]string, 0, len(s.streams))
	for arn := range s.streams {
		arns = append(arns, arn)
	}
	sort.Strings(arns)

	streams := make([]map[string]interface{}, 0, len(arns))
	for _, arn := range arns {
		st := s.streams[arn].stream
		if tableFilter != "" && st.TableName != tableFilter {
			continue
		}
		streams = append(streams, map[string]interface{}{
			"StreamArn":    st.StreamArn,
			"TableName":    st.TableName,
			"StreamLabel":  st.StreamLabel,
			"StreamStatus": st.StreamStatus,
		})
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"Streams": streams})
}

func (s *server) handleDescribeStream(w http.ResponseWriter, payload map[string]interface{}) {
	streamArn, ok := stringField(payload, "StreamArn")
	if !ok || streamArn == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamArn is required")
		return
	}

	s.mu.RLock()
	state, exists := s.streams[streamArn]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	startingSeq := "1"
	if len(state.records) > 0 && state.records[0].SequenceNumber != "" {
		startingSeq = state.records[0].SequenceNumber
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"StreamDescription": map[string]interface{}{
			"StreamArn":    state.stream.StreamArn,
			"TableName":    state.stream.TableName,
			"StreamLabel":  state.stream.StreamLabel,
			"StreamStatus": state.stream.StreamStatus,
			"Shards": []map[string]interface{}{{
				"ShardId": "shard-0001",
				"SequenceNumberRange": map[string]interface{}{
					"StartingSequenceNumber": startingSeq,
				},
			}},
		},
	})
}

func (s *server) handleGetShardIterator(w http.ResponseWriter, payload map[string]interface{}) {
	streamArn, ok := stringField(payload, "StreamArn")
	if !ok || streamArn == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamArn is required")
		return
	}
	shardID, ok := stringField(payload, "ShardId")
	if !ok || shardID == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "ShardId is required")
		return
	}
	if shardID != "shard-0001" {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested shard not found")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.streams[streamArn]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	s.nextIterator++
	it := fmt.Sprintf("mock-iterator:%s:%d", streamArn, s.nextIterator)
	s.iterators[it] = iteratorState{streamArn: streamArn, index: 0}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ShardIterator": it})
}

func (s *server) handleGetRecords(w http.ResponseWriter, payload map[string]interface{}) {
	shardIterator, ok := stringField(payload, "ShardIterator")
	if !ok || strings.TrimSpace(shardIterator) == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "ShardIterator is required")
		return
	}

	limit := 100
	if rawLimit, exists := payload["Limit"]; exists {
		switch v := rawLimit.(type) {
		case float64:
			if int(v) > 0 {
				limit = int(v)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.iterators[shardIterator]
	if !ok {
		writeError(w, http.StatusBadRequest, "ExpiredIteratorException", "ShardIterator is invalid")
		return
	}
	streamState, exists := s.streams[state.streamArn]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	start := state.index
	if start > len(streamState.records) {
		start = len(streamState.records)
	}
	end := start + limit
	if end > len(streamState.records) {
		end = len(streamState.records)
	}

	records := make([]map[string]interface{}, 0, end-start)
	for _, rec := range streamState.records[start:end] {
		records = append(records, map[string]interface{}{
			"dynamodb": map[string]interface{}{
				"SequenceNumber": rec.SequenceNumber,
				"SizeBytes":      rec.SizeBytes,
				"StreamViewType": rec.StreamViewType,
				"NewImage":       rec.NewImage,
				"OldImage":       rec.OldImage,
			},
		})
	}

	nextIndex := end
	s.iterators[shardIterator] = iteratorState{streamArn: state.streamArn, index: nextIndex}
	nextIterator := fmt.Sprintf("%s:%d", shardIterator, nextIndex)
	s.iterators[nextIterator] = iteratorState{streamArn: state.streamArn, index: nextIndex}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Records":           records,
		"NextShardIterator": nextIterator,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, errType, message string) {
	writeJSON(w, status, map[string]interface{}{
		"__type":  errType,
		"message": message,
	})
}

func stringField(payload map[string]interface{}, field string) (string, bool) {
	raw, ok := payload[field]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	return value, ok
}
