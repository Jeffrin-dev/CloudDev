package cloudwatchmetrics

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	jsonContentType = "application/x-amz-json-1.1"
	xmlContentType  = "text/xml"
)

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
		writeXMLError(w, http.StatusMethodNotAllowed, "InvalidParameterValue", "Only POST is supported")
		return
	}

	if err := r.ParseForm(); err == nil {
		action := strings.TrimSpace(r.FormValue("Action"))
		if action != "" {
			s.serveActionXML(w, r, action)
			return
		}
	}

	s.serveLegacyTargetJSON(w, r)
}

func (s *server) serveActionXML(w http.ResponseWriter, r *http.Request, action string) {
	switch action {
	case "PutMetricData":
		s.handlePutMetricDataForm(w, r)
	case "GetMetricStatistics":
		s.handleGetMetricStatisticsForm(w, r)
	case "ListMetrics":
		s.handleListMetricsForm(w, r)
	case "PutMetricAlarm":
		s.handlePutMetricAlarmForm(w, r)
	case "DescribeAlarms":
		s.handleDescribeAlarmsForm(w)
	case "DeleteAlarms":
		s.handleDeleteAlarmsForm(w, r)
	default:
		writeXMLError(w, http.StatusBadRequest, "InvalidAction", "Unknown Action")
	}
}

func (s *server) handlePutMetricDataForm(w http.ResponseWriter, r *http.Request) {
	namespace := strings.TrimSpace(r.FormValue("Namespace"))
	if namespace == "" {
		writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "Namespace is required")
		return
	}

	now := time.Now().UTC()
	batch := make([]MetricDatum, 0)
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("MetricData.member.%d.", i)
		metricName := strings.TrimSpace(r.FormValue(prefix + "MetricName"))
		valueStr := strings.TrimSpace(r.FormValue(prefix + "Value"))
		unit := strings.TrimSpace(r.FormValue(prefix + "Unit"))

		if metricName == "" && valueStr == "" && unit == "" {
			break
		}
		if metricName == "" || valueStr == "" {
			writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "MetricName and Value are required")
			return
		}
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "Metric value must be numeric")
			return
		}

		batch = append(batch, MetricDatum{
			MetricName: metricName,
			Namespace:  namespace,
			Value:      value,
			Timestamp:  now,
			Unit:       unit,
		})
	}

	if len(batch) == 0 {
		writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "MetricData is required")
		return
	}

	s.mu.Lock()
	s.metrics = append(s.metrics, batch...)
	s.mu.Unlock()

	writeXML(w, http.StatusOK, putMetricDataResponse{XMLNS: awsCloudWatchNamespace})
}

func (s *server) handleGetMetricStatisticsForm(w http.ResponseWriter, r *http.Request) {
	namespace := strings.TrimSpace(r.FormValue("Namespace"))
	metricName := strings.TrimSpace(r.FormValue("MetricName"))
	if namespace == "" || metricName == "" {
		writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "Namespace and MetricName are required")
		return
	}

	s.mu.RLock()
	filtered := make([]MetricDatum, 0)
	for _, metric := range s.metrics {
		if metric.Namespace == namespace && metric.MetricName == metricName {
			filtered = append(filtered, metric)
		}
	}
	s.mu.RUnlock()

	stats := statisticsDatum{}
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
		stats = statisticsDatum{
			SampleCount: sampleCount,
			Sum:         sum,
			Average:     sum / sampleCount,
			Minimum:     minVal,
			Maximum:     maxVal,
		}
	}

	writeXML(w, http.StatusOK, getMetricStatisticsResponse{
		XMLNS: awsCloudWatchNamespace,
		Datapoints: datapoints{
			Members: []statisticsDatum{stats},
		},
	})
}

func (s *server) handleListMetricsForm(w http.ResponseWriter, r *http.Request) {
	namespaceFilter := strings.TrimSpace(r.FormValue("Namespace"))

	s.mu.RLock()
	seen := make(map[string]struct{})
	metrics := make([]metricMember, 0)
	for _, metric := range s.metrics {
		if namespaceFilter != "" && namespaceFilter != metric.Namespace {
			continue
		}
		key := metric.Namespace + "|" + metric.MetricName
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		metrics = append(metrics, metricMember{MetricName: metric.MetricName, Namespace: metric.Namespace})
	}
	s.mu.RUnlock()

	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Namespace == metrics[j].Namespace {
			return metrics[i].MetricName < metrics[j].MetricName
		}
		return metrics[i].Namespace < metrics[j].Namespace
	})

	writeXML(w, http.StatusOK, listMetricsResponse{XMLNS: awsCloudWatchNamespace, Metrics: metricsList{Members: metrics}})
}

func (s *server) handlePutMetricAlarmForm(w http.ResponseWriter, r *http.Request) {
	alarmName := strings.TrimSpace(r.FormValue("AlarmName"))
	metricName := strings.TrimSpace(r.FormValue("MetricName"))
	namespace := strings.TrimSpace(r.FormValue("Namespace"))
	thresholdStr := strings.TrimSpace(r.FormValue("Threshold"))
	comparisonOperator := strings.TrimSpace(r.FormValue("ComparisonOperator"))

	if alarmName == "" || metricName == "" || namespace == "" || thresholdStr == "" || comparisonOperator == "" {
		writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "AlarmName, MetricName, Namespace, Threshold, and ComparisonOperator are required")
		return
	}

	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "Threshold must be numeric")
		return
	}

	s.mu.Lock()
	s.alarms[alarmName] = Alarm{
		AlarmName:          alarmName,
		MetricName:         metricName,
		Namespace:          namespace,
		Threshold:          threshold,
		ComparisonOperator: comparisonOperator,
		State:              "OK",
	}
	s.mu.Unlock()

	writeXML(w, http.StatusOK, putMetricAlarmResponse{XMLNS: awsCloudWatchNamespace})
}

func (s *server) handleDescribeAlarmsForm(w http.ResponseWriter) {
	s.mu.RLock()
	alarms := make([]alarmMember, 0, len(s.alarms))
	for _, alarm := range s.alarms {
		alarms = append(alarms, alarmMember{
			AlarmName:          alarm.AlarmName,
			MetricName:         alarm.MetricName,
			Namespace:          alarm.Namespace,
			Threshold:          alarm.Threshold,
			ComparisonOperator: alarm.ComparisonOperator,
			StateValue:         alarm.State,
		})
	}
	s.mu.RUnlock()

	sort.Slice(alarms, func(i, j int) bool {
		return alarms[i].AlarmName < alarms[j].AlarmName
	})

	writeXML(w, http.StatusOK, describeAlarmsResponse{XMLNS: awsCloudWatchNamespace, MetricAlarms: alarmList{Members: alarms}})
}

func (s *server) handleDeleteAlarmsForm(w http.ResponseWriter, r *http.Request) {
	names := make([]string, 0)
	for i := 1; ; i++ {
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("AlarmNames.member.%d", i)))
		if name == "" {
			break
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		writeXMLError(w, http.StatusBadRequest, "InvalidParameterValue", "AlarmNames is required")
		return
	}

	s.mu.Lock()
	for _, name := range names {
		delete(s.alarms, name)
	}
	s.mu.Unlock()

	writeXML(w, http.StatusOK, deleteAlarmsResponse{XMLNS: awsCloudWatchNamespace})
}

func (s *server) serveLegacyTargetJSON(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.Header.Get("X-Amz-Target"))
	if target == "" {
		writeXMLError(w, http.StatusBadRequest, "MissingAction", "Action is required")
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
		batch = append(batch, MetricDatum{
			MetricName: metricName,
			Namespace:  namespace,
			Value:      value,
			Timestamp:  now,
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

	s.mu.Lock()
	s.alarms[alarmName] = Alarm{
		AlarmName:          alarmName,
		MetricName:         metricName,
		Namespace:          namespace,
		Threshold:          threshold,
		ComparisonOperator: comparisonOperator,
		State:              "OK",
	}
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

const awsCloudWatchNamespace = "http://monitoring.amazonaws.com/doc/2010-08-01/"

type putMetricDataResponse struct {
	XMLName xml.Name `xml:"PutMetricDataResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type statisticsDatum struct {
	SampleCount float64 `xml:"SampleCount"`
	Sum         float64 `xml:"Sum"`
	Average     float64 `xml:"Average"`
	Minimum     float64 `xml:"Minimum"`
	Maximum     float64 `xml:"Maximum"`
}

type datapoints struct {
	Members []statisticsDatum `xml:"member"`
}

type getMetricStatisticsResponse struct {
	XMLName    xml.Name   `xml:"GetMetricStatisticsResponse"`
	XMLNS      string     `xml:"xmlns,attr"`
	Datapoints datapoints `xml:"GetMetricStatisticsResult>Datapoints"`
}

type metricMember struct {
	MetricName string `xml:"MetricName"`
	Namespace  string `xml:"Namespace"`
}

type metricsList struct {
	Members []metricMember `xml:"member"`
}

type listMetricsResponse struct {
	XMLName xml.Name    `xml:"ListMetricsResponse"`
	XMLNS   string      `xml:"xmlns,attr"`
	Metrics metricsList `xml:"ListMetricsResult>Metrics"`
}

type putMetricAlarmResponse struct {
	XMLName xml.Name `xml:"PutMetricAlarmResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type alarmMember struct {
	AlarmName          string  `xml:"AlarmName"`
	MetricName         string  `xml:"MetricName"`
	Namespace          string  `xml:"Namespace"`
	Threshold          float64 `xml:"Threshold"`
	ComparisonOperator string  `xml:"ComparisonOperator"`
	StateValue         string  `xml:"StateValue"`
}

type alarmList struct {
	Members []alarmMember `xml:"member"`
}

type describeAlarmsResponse struct {
	XMLName      xml.Name  `xml:"DescribeAlarmsResponse"`
	XMLNS        string    `xml:"xmlns,attr"`
	MetricAlarms alarmList `xml:"DescribeAlarmsResult>MetricAlarms"`
}

type deleteAlarmsResponse struct {
	XMLName xml.Name `xml:"DeleteAlarmsResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type errorResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Error   struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Error"`
}

func writeXML(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(payload)
}

func writeXMLError(w http.ResponseWriter, status int, code, message string) {
	resp := errorResponse{}
	resp.Error.Type = "Sender"
	resp.Error.Code = code
	resp.Error.Message = message
	writeXML(w, status, resp)
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
