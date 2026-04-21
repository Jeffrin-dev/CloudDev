package kinesis

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

type Stream struct {
	StreamName   string
	StreamARN    string
	StreamStatus string
	Shards       []Shard
}

type Shard struct {
	ShardId string
}

type Record struct {
	SequenceNumber string
	Data           string
	PartitionKey   string
}

type streamState struct {
	stream       Stream
	records      map[string][]Record
	nextSequence int64
}

type server struct {
	mu      sync.RWMutex
	streams map[string]*streamState
}

func newServer() *server {
	return &server{streams: make(map[string]*streamState)}
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
	case "Kinesis_20131202.CreateStream":
		s.handleCreateStream(w, payload)
	case "Kinesis_20131202.DeleteStream":
		s.handleDeleteStream(w, payload)
	case "Kinesis_20131202.ListStreams":
		s.handleListStreams(w)
	case "Kinesis_20131202.DescribeStream":
		s.handleDescribeStream(w, payload)
	case "Kinesis_20131202.PutRecord":
		s.handlePutRecord(w, payload)
	case "Kinesis_20131202.PutRecords":
		s.handlePutRecords(w, payload)
	case "Kinesis_20131202.GetShardIterator":
		s.handleGetShardIterator(w, payload)
	case "Kinesis_20131202.GetRecords":
		s.handleGetRecords(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateStream(w http.ResponseWriter, payload map[string]interface{}) {
	streamName, ok := stringField(payload, "StreamName")
	if !ok || streamName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamName is required")
		return
	}

	shardCount, ok := intField(payload, "ShardCount")
	if !ok || shardCount <= 0 {
		writeError(w, http.StatusBadRequest, "ValidationException", "ShardCount must be greater than zero")
		return
	}

	shards := make([]Shard, 0, shardCount)
	for i := 0; i < shardCount; i++ {
		shards = append(shards, Shard{ShardId: fmt.Sprintf("shardId-%012d", i)})
	}

	stream := Stream{
		StreamName:   streamName,
		StreamARN:    streamARN(streamName),
		StreamStatus: "ACTIVE",
		Shards:       shards,
	}

	s.mu.Lock()
	s.streams[streamName] = &streamState{
		stream:  stream,
		records: make(map[string][]Record),
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleDeleteStream(w http.ResponseWriter, payload map[string]interface{}) {
	streamName, ok := stringField(payload, "StreamName")
	if !ok || streamName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamName is required")
		return
	}

	s.mu.Lock()
	delete(s.streams, streamName)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleListStreams(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]string, 0, len(s.streams))
	for name := range s.streams {
		names = append(names, name)
	}
	s.mu.RUnlock()

	sort.Strings(names)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"StreamNames":    names,
		"HasMoreStreams": false,
	})
}

func (s *server) handleDescribeStream(w http.ResponseWriter, payload map[string]interface{}) {
	streamName, ok := stringField(payload, "StreamName")
	if !ok || streamName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamName is required")
		return
	}

	s.mu.RLock()
	state, exists := s.streams[streamName]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Stream does not exist")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"StreamDescription": state.stream,
	})
}

func (s *server) handlePutRecord(w http.ResponseWriter, payload map[string]interface{}) {
	streamName, ok := stringField(payload, "StreamName")
	if !ok || streamName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamName is required")
		return
	}
	data, ok := stringField(payload, "Data")
	if !ok || data == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Data is required")
		return
	}
	partitionKey, ok := stringField(payload, "PartitionKey")
	if !ok || partitionKey == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "PartitionKey is required")
		return
	}

	s.mu.Lock()
	state, exists := s.streams[streamName]
	if !exists {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Stream does not exist")
		return
	}

	shardID := chooseShard(state.stream.Shards, partitionKey)
	state.nextSequence++
	sequenceNumber := strconv.FormatInt(state.nextSequence, 10)
	rec := Record{SequenceNumber: sequenceNumber, Data: data, PartitionKey: partitionKey}
	state.records[shardID] = append(state.records[shardID], rec)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"SequenceNumber": sequenceNumber,
		"ShardId":        shardID,
	})
}

func (s *server) handlePutRecords(w http.ResponseWriter, payload map[string]interface{}) {
	streamName, ok := stringField(payload, "StreamName")
	if !ok || streamName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamName is required")
		return
	}

	recordsPayload, ok := payload["Records"].([]interface{})
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Records is required")
		return
	}

	s.mu.Lock()
	state, exists := s.streams[streamName]
	if !exists {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Stream does not exist")
		return
	}

	results := make([]map[string]interface{}, 0, len(recordsPayload))
	for _, item := range recordsPayload {
		recordMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		data, dataOK := stringField(recordMap, "Data")
		partitionKey, keyOK := stringField(recordMap, "PartitionKey")
		if !dataOK || !keyOK || data == "" || partitionKey == "" {
			continue
		}
		shardID := chooseShard(state.stream.Shards, partitionKey)
		state.nextSequence++
		sequenceNumber := strconv.FormatInt(state.nextSequence, 10)
		rec := Record{SequenceNumber: sequenceNumber, Data: data, PartitionKey: partitionKey}
		state.records[shardID] = append(state.records[shardID], rec)
		results = append(results, map[string]interface{}{
			"SequenceNumber": sequenceNumber,
			"ShardId":        shardID,
		})
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"FailedRecordCount": 0,
		"Records":           results,
	})
}

func (s *server) handleGetShardIterator(w http.ResponseWriter, payload map[string]interface{}) {
	streamName, ok := stringField(payload, "StreamName")
	if !ok || streamName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "StreamName is required")
		return
	}
	shardID, ok := stringField(payload, "ShardId")
	if !ok || shardID == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "ShardId is required")
		return
	}

	s.mu.RLock()
	state, exists := s.streams[streamName]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Stream does not exist")
		return
	}
	if !hasShard(state.stream.Shards, shardID) {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Shard does not exist")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ShardIterator": fmt.Sprintf("%s:%s:0", streamName, shardID),
	})
}

func (s *server) handleGetRecords(w http.ResponseWriter, payload map[string]interface{}) {
	iterator, ok := stringField(payload, "ShardIterator")
	if !ok || iterator == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "ShardIterator is required")
		return
	}

	parts := strings.Split(iterator, ":")
	if len(parts) != 3 {
		writeError(w, http.StatusBadRequest, "ValidationException", "Invalid ShardIterator")
		return
	}
	streamName := parts[0]
	shardID := parts[1]
	start, err := strconv.Atoi(parts[2])
	if err != nil || start < 0 {
		writeError(w, http.StatusBadRequest, "ValidationException", "Invalid ShardIterator index")
		return
	}

	limit := 100
	if v, ok := intField(payload, "Limit"); ok && v > 0 {
		limit = v
	}

	s.mu.RLock()
	state, exists := s.streams[streamName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Stream does not exist")
		return
	}
	shardRecords := state.records[shardID]
	if start > len(shardRecords) {
		start = len(shardRecords)
	}
	end := start + limit
	if end > len(shardRecords) {
		end = len(shardRecords)
	}
	out := append([]Record(nil), shardRecords[start:end]...)
	next := fmt.Sprintf("%s:%s:%d", streamName, shardID, end)
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Records":           out,
		"NextShardIterator": next,
	})
}

func chooseShard(shards []Shard, partitionKey string) string {
	if len(shards) == 0 {
		return "shardId-000000000000"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(partitionKey))
	idx := int(h.Sum32() % uint32(len(shards)))
	return shards[idx].ShardId
}

func hasShard(shards []Shard, shardID string) bool {
	for _, shard := range shards {
		if shard.ShardId == shardID {
			return true
		}
	}
	return false
}

func streamARN(streamName string) string {
	return fmt.Sprintf("arn:aws:kinesis:%s:%s:stream/%s", region, accountID, streamName)
}

func stringField(payload map[string]interface{}, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	return s, ok
}

func intField(payload map[string]interface{}, key string) (int, bool) {
	value, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"__type":  code,
		"message": message,
	})
}
