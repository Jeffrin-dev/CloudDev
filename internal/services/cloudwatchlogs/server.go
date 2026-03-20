package cloudwatchlogs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const jsonContentType = "application/x-amz-json-1.1"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

type LogGroup struct {
	LogGroupName string
	CreationTime int64
	Streams      map[string]*LogStream
}

type LogStream struct {
	LogStreamName string
	CreationTime  int64
	Events        []LogEvent
}

type LogEvent struct {
	Timestamp     int64
	Message       string
	IngestionTime int64
}

type server struct {
	mu     sync.RWMutex
	groups map[string]*LogGroup
}

func newServer() *server {
	return &server{groups: make(map[string]*LogGroup)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "InvalidParameterException", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Missing X-Amz-Target header")
		return
	}

	payload := map[string]any{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, "SerializationException", "Invalid JSON body")
			return
		}
	}

	switch target {
	case "Logs_20140328.CreateLogGroup":
		s.handleCreateLogGroup(w, payload)
	case "Logs_20140328.DeleteLogGroup":
		s.handleDeleteLogGroup(w, payload)
	case "Logs_20140328.DescribeLogGroups":
		s.handleDescribeLogGroups(w, payload)
	case "Logs_20140328.CreateLogStream":
		s.handleCreateLogStream(w, payload)
	case "Logs_20140328.DeleteLogStream":
		s.handleDeleteLogStream(w, payload)
	case "Logs_20140328.DescribeLogStreams":
		s.handleDescribeLogStreams(w, payload)
	case "Logs_20140328.PutLogEvents":
		s.handlePutLogEvents(w, payload)
	case "Logs_20140328.GetLogEvents":
		s.handleGetLogEvents(w, payload)
	case "Logs_20140328.FilterLogEvents":
		s.handleFilterLogEvents(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateLogGroup(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.groups[name]; exists {
		writeError(w, http.StatusBadRequest, "ResourceAlreadyExistsException", "The specified log group already exists")
		return
	}

	s.groups[name] = &LogGroup{LogGroupName: name, CreationTime: nowMillis(), Streams: make(map[string]*LogStream)}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) handleDeleteLogGroup(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.groups[name]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	delete(s.groups, name)
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) handleDescribeLogGroups(w http.ResponseWriter, payload map[string]any) {
	prefix, _ := stringField(payload, "logGroupNamePrefix")

	s.mu.RLock()
	groups := make([]map[string]any, 0, len(s.groups))
	for _, group := range s.groups {
		if prefix != "" && !strings.HasPrefix(group.LogGroupName, prefix) {
			continue
		}
		groups = append(groups, map[string]any{
			"arn":               logGroupARN(group.LogGroupName),
			"creationTime":      group.CreationTime,
			"logGroupName":      group.LogGroupName,
			"storedBytes":       0,
			"metricFilterCount": 0,
		})
	}
	s.mu.RUnlock()

	sort.Slice(groups, func(i, j int) bool {
		return groups[i]["logGroupName"].(string) < groups[j]["logGroupName"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"logGroups": groups})
}

func (s *server) handleCreateLogStream(w http.ResponseWriter, payload map[string]any) {
	groupName, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}
	streamName, ok := stringField(payload, "logStreamName")
	if !ok || strings.TrimSpace(streamName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logStreamName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	group, exists := s.groups[groupName]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	if _, exists := group.Streams[streamName]; exists {
		writeError(w, http.StatusBadRequest, "ResourceAlreadyExistsException", "The specified log stream already exists")
		return
	}

	group.Streams[streamName] = &LogStream{LogStreamName: streamName, CreationTime: nowMillis(), Events: []LogEvent{}}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) handleDeleteLogStream(w http.ResponseWriter, payload map[string]any) {
	groupName, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}
	streamName, ok := stringField(payload, "logStreamName")
	if !ok || strings.TrimSpace(streamName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logStreamName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	group, exists := s.groups[groupName]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	if _, exists := group.Streams[streamName]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log stream does not exist")
		return
	}
	delete(group.Streams, streamName)
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) handleDescribeLogStreams(w http.ResponseWriter, payload map[string]any) {
	groupName, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}
	prefix, _ := stringField(payload, "logStreamNamePrefix")

	s.mu.RLock()
	group, exists := s.groups[groupName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	streams := make([]map[string]any, 0, len(group.Streams))
	for _, stream := range group.Streams {
		if prefix != "" && !strings.HasPrefix(stream.LogStreamName, prefix) {
			continue
		}
		lastIngestion := int64(0)
		lastEvent := int64(0)
		storedBytes := 0
		if len(stream.Events) > 0 {
			last := stream.Events[len(stream.Events)-1]
			lastIngestion = last.IngestionTime
			lastEvent = last.Timestamp
			for _, event := range stream.Events {
				storedBytes += len(event.Message)
			}
		}
		streams = append(streams, map[string]any{
			"arn":                 logStreamARN(groupName, stream.LogStreamName),
			"creationTime":        stream.CreationTime,
			"logStreamName":       stream.LogStreamName,
			"storedBytes":         storedBytes,
			"lastEventTimestamp":  lastEvent,
			"lastIngestionTime":   lastIngestion,
			"uploadSequenceToken": "0",
		})
	}
	s.mu.RUnlock()

	sort.Slice(streams, func(i, j int) bool {
		return streams[i]["logStreamName"].(string) < streams[j]["logStreamName"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"logStreams": streams})
}

func (s *server) handlePutLogEvents(w http.ResponseWriter, payload map[string]any) {
	groupName, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}
	streamName, ok := stringField(payload, "logStreamName")
	if !ok || strings.TrimSpace(streamName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logStreamName is required")
		return
	}
	rawEvents, ok := payload["logEvents"].([]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logEvents is required")
		return
	}

	ingestionTime := nowMillis()
	events := make([]LogEvent, 0, len(rawEvents))
	for _, rawEvent := range rawEvents {
		eventMap, ok := rawEvent.(map[string]any)
		if !ok {
			writeError(w, http.StatusBadRequest, "InvalidParameterException", "logEvents must contain valid event objects")
			return
		}
		timestamp, ok := int64Field(eventMap, "timestamp")
		if !ok {
			writeError(w, http.StatusBadRequest, "InvalidParameterException", "logEvents timestamp is required")
			return
		}
		message, ok := stringField(eventMap, "message")
		if !ok {
			writeError(w, http.StatusBadRequest, "InvalidParameterException", "logEvents message is required")
			return
		}
		events = append(events, LogEvent{Timestamp: timestamp, Message: message, IngestionTime: ingestionTime})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Timestamp < events[j].Timestamp })

	s.mu.Lock()
	defer s.mu.Unlock()
	group, exists := s.groups[groupName]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	stream, exists := group.Streams[streamName]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log stream does not exist")
		return
	}
	stream.Events = append(stream.Events, events...)
	sort.Slice(stream.Events, func(i, j int) bool { return stream.Events[i].Timestamp < stream.Events[j].Timestamp })

	writeJSON(w, http.StatusOK, map[string]any{"nextSequenceToken": "0"})
}

func (s *server) handleGetLogEvents(w http.ResponseWriter, payload map[string]any) {
	groupName, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}
	streamName, ok := stringField(payload, "logStreamName")
	if !ok || strings.TrimSpace(streamName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logStreamName is required")
		return
	}
	startTime, hasStart := int64Field(payload, "startTime")
	endTime, hasEnd := int64Field(payload, "endTime")

	s.mu.RLock()
	group, exists := s.groups[groupName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	stream, exists := group.Streams[streamName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log stream does not exist")
		return
	}
	events := filterEvents(stream.Events, hasStart, startTime, hasEnd, endTime, "")
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"events": toEventPayload(events), "nextBackwardToken": "", "nextForwardToken": ""})
}

func (s *server) handleFilterLogEvents(w http.ResponseWriter, payload map[string]any) {
	groupName, ok := stringField(payload, "logGroupName")
	if !ok || strings.TrimSpace(groupName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "logGroupName is required")
		return
	}
	pattern, _ := stringField(payload, "filterPattern")
	startTime, hasStart := int64Field(payload, "startTime")
	endTime, hasEnd := int64Field(payload, "endTime")

	s.mu.RLock()
	group, exists := s.groups[groupName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "The specified log group does not exist")
		return
	}
	filtered := make([]map[string]any, 0)
	for _, stream := range group.Streams {
		for _, event := range filterEvents(stream.Events, hasStart, startTime, hasEnd, endTime, pattern) {
			filtered = append(filtered, map[string]any{
				"eventId":       fmt.Sprintf("%s:%s:%d", groupName, stream.LogStreamName, event.Timestamp),
				"ingestionTime": event.IngestionTime,
				"logStreamName": stream.LogStreamName,
				"message":       event.Message,
				"timestamp":     event.Timestamp,
			})
		}
	}
	s.mu.RUnlock()

	sort.Slice(filtered, func(i, j int) bool {
		left := filtered[i]["timestamp"].(int64)
		right := filtered[j]["timestamp"].(int64)
		if left == right {
			return filtered[i]["logStreamName"].(string) < filtered[j]["logStreamName"].(string)
		}
		return left < right
	})

	writeJSON(w, http.StatusOK, map[string]any{"events": filtered, "searchedLogStreams": []map[string]any{}})
}

func filterEvents(events []LogEvent, hasStart bool, start int64, hasEnd bool, end int64, pattern string) []LogEvent {
	filtered := make([]LogEvent, 0, len(events))
	for _, event := range events {
		if hasStart && event.Timestamp < start {
			continue
		}
		if hasEnd && event.Timestamp > end {
			continue
		}
		if pattern != "" && !strings.Contains(event.Message, pattern) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func toEventPayload(events []LogEvent) []map[string]any {
	result := make([]map[string]any, 0, len(events))
	for _, event := range events {
		result = append(result, map[string]any{
			"ingestionTime": event.IngestionTime,
			"message":       event.Message,
			"timestamp":     event.Timestamp,
		})
	}
	return result
}

func stringField(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

func int64Field(payload map[string]any, key string) (int64, bool) {
	value, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

func logGroupARN(name string) string {
	return fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", region, accountID, name)
}

func logStreamARN(groupName, streamName string) string {
	return fmt.Sprintf("%s:log-stream:%s", logGroupARN(groupName), streamName)
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"__type": code, "message": message})
}
