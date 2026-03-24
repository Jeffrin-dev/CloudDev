package cloudwatchmetrics

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const jsonContentType = "application/x-amz-json-1.1"

type MetricDatum struct {
	MetricName string
	Namespace  string
	Value      float64
	Timestamp  time.Time
	Unit       string
}

type Alarm struct {
	AlarmName          string
	MetricName         string
	Namespace          string
	Threshold          float64
	ComparisonOperator string
	State              string
}

type server struct {
	mu      sync.RWMutex
	metrics []MetricDatum
	alarms  map[string]Alarm
}

func newServer() *server {
	return &server{
		metrics: make([]MetricDatum, 0),
		alarms:  make(map[string]Alarm),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "InvalidParameterException", "Only POST is supported")
		return
	}

	target := strings.TrimSpace(r.Header.Get("X-Amz-Target"))
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
	case "GraniteServiceVersion20100801.PutMetricData":
		s.handlePutMetricData(w, payload)
	case "GraniteServiceVersion20100801.GetMetricStatistics":
		s.handleGetMetricStatistics(w, payload)
	case "GraniteServiceVersion20100801.ListMetrics":
		s.handleListMetrics(w)
	case "GraniteServiceVersion20100801.PutMetricAlarm":
		s.handlePutMetricAlarm(w, payload)
	case "GraniteServiceVersion20100801.DescribeAlarms":
		s.handleDescribeAlarms(w)
	case "GraniteServiceVersion20100801.DeleteAlarms":
		s.handleDeleteAlarms(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handlePutMetricData(w http.ResponseWriter, payload map[string]any) {
	metricData, ok := payload["MetricData"].([]any)
	if !ok || len(metricData) == 0 {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "MetricData is required")
		return
	}

	namespace, ok := stringField(payload, "Namespace")
	if !ok || strings.TrimSpace(namespace) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Namespace is required")
		return
	}

	now := time.Now().UTC()
	batch := make([]MetricDatum, 0, len(metricData))
	for _, item := range metricData {
		datumMap, ok := item.(map[string]any)
		if !ok {
			writeError(w, http.StatusBadRequest, "SerializationException", "MetricData entry must be an object")
			return
		}
		metricName, ok := stringField(datumMap, "MetricName")
		if !ok || strings.TrimSpace(metricName) == "" {
			writeError(w, http.StatusBadRequest, "InvalidParameterValue", "MetricName is required")
			return
		}
		value, ok := floatField(datumMap, "Value")
		if !ok {
			writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Value is required")
			return
		}
		unit, _ := stringField(datumMap, "Unit")
		timestamp := now
		if rawTimestamp, exists := datumMap["Timestamp"]; exists {
			parsed, err := parseTimestamp(rawTimestamp)
			if err != nil {
				writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Timestamp must be RFC3339")
				return
			}
			timestamp = parsed
		}
		batch = append(batch, MetricDatum{
			MetricName: metricName,
			Namespace:  namespace,
			Value:      value,
			Timestamp:  timestamp,
			Unit:       unit,
		})
	}

	s.mu.Lock()
	s.metrics = append(s.metrics, batch...)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) handleGetMetricStatistics(w http.ResponseWriter, payload map[string]any) {
	metricName, ok := stringField(payload, "MetricName")
	if !ok || strings.TrimSpace(metricName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "MetricName is required")
		return
	}
	namespace, ok := stringField(payload, "Namespace")
	if !ok || strings.TrimSpace(namespace) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Namespace is required")
		return
	}

	s.mu.RLock()
	filtered := make([]MetricDatum, 0)
	for _, metric := range s.metrics {
		if metric.MetricName == metricName && metric.Namespace == namespace {
			filtered = append(filtered, metric)
		}
	}
	s.mu.RUnlock()

	stats := map[string]any{"SampleCount": float64(0), "Sum": float64(0), "Average": float64(0), "Minimum": float64(0), "Maximum": float64(0)}
	if len(filtered) > 0 {
		sum := 0.0
		minVal := math.MaxFloat64
		maxVal := -math.MaxFloat64
		for _, datum := range filtered {
			sum += datum.Value
			if datum.Value < minVal {
				minVal = datum.Value
			}
			if datum.Value > maxVal {
				maxVal = datum.Value
			}
		}
		sampleCount := float64(len(filtered))
		stats = map[string]any{
			"SampleCount": sampleCount,
			"Sum":         sum,
			"Average":     sum / sampleCount,
			"Minimum":     minVal,
			"Maximum":     maxVal,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"Datapoints": []map[string]any{stats}})
}

func (s *server) handleListMetrics(w http.ResponseWriter) {
	s.mu.RLock()
	seen := make(map[string]struct{})
	metrics := make([]map[string]any, 0)
	for _, metric := range s.metrics {
		key := metric.Namespace + "|" + metric.MetricName
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		metrics = append(metrics, map[string]any{
			"Namespace":  metric.Namespace,
			"MetricName": metric.MetricName,
		})
	}
	s.mu.RUnlock()

	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i]["Namespace"].(string) == metrics[j]["Namespace"].(string) {
			return metrics[i]["MetricName"].(string) < metrics[j]["MetricName"].(string)
		}
		return metrics[i]["Namespace"].(string) < metrics[j]["Namespace"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"Metrics": metrics})
}

func (s *server) handlePutMetricAlarm(w http.ResponseWriter, payload map[string]any) {
	alarmName, ok := stringField(payload, "AlarmName")
	if !ok || strings.TrimSpace(alarmName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "AlarmName is required")
		return
	}
	metricName, ok := stringField(payload, "MetricName")
	if !ok || strings.TrimSpace(metricName) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "MetricName is required")
		return
	}
	namespace, ok := stringField(payload, "Namespace")
	if !ok || strings.TrimSpace(namespace) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Namespace is required")
		return
	}
	threshold, ok := floatField(payload, "Threshold")
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Threshold is required")
		return
	}
	comparisonOperator, ok := stringField(payload, "ComparisonOperator")
	if !ok || strings.TrimSpace(comparisonOperator) == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "ComparisonOperator is required")
		return
	}

	alarm := Alarm{
		AlarmName:          alarmName,
		MetricName:         metricName,
		Namespace:          namespace,
		Threshold:          threshold,
		ComparisonOperator: comparisonOperator,
		State:              "OK",
	}

	s.mu.Lock()
	s.alarms[alarmName] = alarm
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) handleDescribeAlarms(w http.ResponseWriter) {
	s.mu.RLock()
	alarms := make([]map[string]any, 0, len(s.alarms))
	for _, alarm := range s.alarms {
		alarms = append(alarms, map[string]any{
			"AlarmName":          alarm.AlarmName,
			"MetricName":         alarm.MetricName,
			"Namespace":          alarm.Namespace,
			"Threshold":          alarm.Threshold,
			"ComparisonOperator": alarm.ComparisonOperator,
			"StateValue":         alarm.State,
		})
	}
	s.mu.RUnlock()

	sort.Slice(alarms, func(i, j int) bool {
		return alarms[i]["AlarmName"].(string) < alarms[j]["AlarmName"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"MetricAlarms": alarms})
}

func (s *server) handleDeleteAlarms(w http.ResponseWriter, payload map[string]any) {
	rawNames, ok := payload["AlarmNames"].([]any)
	if !ok || len(rawNames) == 0 {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "AlarmNames is required")
		return
	}

	s.mu.Lock()
	for _, rawName := range rawNames {
		name, ok := rawName.(string)
		if !ok {
			continue
		}
		delete(s.alarms, name)
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{})
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"__type": code, "message": message})
}

func stringField(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	asString, ok := value.(string)
	if !ok {
		return "", false
	}
	return asString, true
}

func floatField(payload map[string]any, key string) (float64, bool) {
	value, ok := payload[key]
	if !ok {
		return 0, false
	}
	asFloat, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return asFloat, true
}

func parseTimestamp(raw any) (time.Time, error) {
	value, ok := raw.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("timestamp must be a string")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}
