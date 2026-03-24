package eventbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const jsonContentType = "application/x-amz-json-1.1"

const (
	region          = "us-east-1"
	accountID       = "000000000000"
	defaultEventBus = "default"
)

type EventBus struct {
	Name string
	Arn  string
}

type Target struct {
	ID  string `json:"Id"`
	Arn string `json:"Arn"`
}

type Rule struct {
	Name         string
	EventBusName string
	EventPattern string
	State        string
	Targets      []Target
}

type server struct {
	mu         sync.RWMutex
	lambdaPort int
	sqsPort    int
	eventBuses map[string]EventBus
	rules      map[string]Rule
}

func newServer(lambdaPort int, sqsPort int) *server {
	s := &server{
		lambdaPort: lambdaPort,
		sqsPort:    sqsPort,
		eventBuses: make(map[string]EventBus),
		rules:      make(map[string]Rule),
	}
	s.eventBuses[defaultEventBus] = EventBus{Name: defaultEventBus, Arn: eventBusARN(defaultEventBus)}
	return s
}

func Start(port int, lambdaPort int, sqsPort int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer(lambdaPort, sqsPort))
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "Missing X-Amz-Target header")
		return
	}

	payload := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterValue", "Invalid JSON body")
		return
	}

	switch target {
	case "AmazonEventBridge.CreateEventBus":
		s.handleCreateEventBus(w, payload)
	case "AmazonEventBridge.ListEventBuses":
		s.handleListEventBuses(w)
	case "AmazonEventBridge.PutRule":
		s.handlePutRule(w, payload)
	case "AmazonEventBridge.ListRules":
		s.handleListRules(w, payload)
	case "AmazonEventBridge.PutTargets":
		s.handlePutTargets(w, payload)
	case "AmazonEventBridge.PutEvents":
		s.handlePutEvents(w, payload)
	case "AmazonEventBridge.DeleteRule":
		s.handleDeleteRule(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateEventBus(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	bus, exists := s.eventBuses[name]
	if !exists {
		bus = EventBus{Name: name, Arn: eventBusARN(name)}
		s.eventBuses[name] = bus
	}

	writeJSON(w, http.StatusOK, map[string]any{"EventBusArn": bus.Arn})
}

func (s *server) handleListEventBuses(w http.ResponseWriter) {
	s.mu.RLock()
	buses := make([]EventBus, 0, len(s.eventBuses))
	for _, bus := range s.eventBuses {
		buses = append(buses, bus)
	}
	s.mu.RUnlock()

	sort.Slice(buses, func(i, j int) bool { return buses[i].Name < buses[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"EventBuses": buses})
}

func (s *server) handlePutRule(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}
	eventPattern, ok := stringField(payload, "EventPattern")
	if !ok || eventPattern == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "EventPattern is required")
		return
	}
	eventBusName, _ := stringField(payload, "EventBusName")
	if eventBusName == "" {
		eventBusName = defaultEventBus
	}
	state, _ := stringField(payload, "State")
	if state == "" {
		state = "ENABLED"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.eventBuses[eventBusName]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Event bus does not exist")
		return
	}

	key := ruleKey(eventBusName, name)
	rule := s.rules[key]
	rule.Name = name
	rule.EventBusName = eventBusName
	rule.EventPattern = eventPattern
	rule.State = state
	s.rules[key] = rule

	writeJSON(w, http.StatusOK, map[string]any{"RuleArn": ruleARN(eventBusName, name)})
}

func (s *server) handleListRules(w http.ResponseWriter, payload map[string]any) {
	eventBusName, _ := stringField(payload, "EventBusName")
	if eventBusName == "" {
		eventBusName = defaultEventBus
	}

	s.mu.RLock()
	rules := make([]Rule, 0)
	for _, rule := range s.rules {
		if rule.EventBusName == eventBusName {
			rules = append(rules, rule)
		}
	}
	s.mu.RUnlock()

	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"Rules": rules})
}

func (s *server) handlePutTargets(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "Rule")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Rule is required")
		return
	}
	eventBusName, _ := stringField(payload, "EventBusName")
	if eventBusName == "" {
		eventBusName = defaultEventBus
	}
	rawTargets, ok := payload["Targets"].([]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Targets is required")
		return
	}
	targets := make([]Target, 0, len(rawTargets))
	for _, rt := range rawTargets {
		item, ok := rt.(map[string]any)
		if !ok {
			continue
		}
		id, _ := stringField(item, "Id")
		arn, _ := stringField(item, "Arn")
		if id == "" || arn == "" {
			continue
		}
		targets = append(targets, Target{ID: id, Arn: arn})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := ruleKey(eventBusName, name)
	rule, exists := s.rules[key]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Rule does not exist")
		return
	}
	rule.Targets = targets
	s.rules[key] = rule

	writeJSON(w, http.StatusOK, map[string]any{"FailedEntryCount": 0, "FailedEntries": []any{}})
}

func (s *server) handlePutEvents(w http.ResponseWriter, payload map[string]any) {
	rawEntries, ok := payload["Entries"].([]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Entries is required")
		return
	}

	entries := make([]map[string]any, 0, len(rawEntries))
	for _, re := range rawEntries {
		entry, ok := re.(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	responses := make([]map[string]any, 0, len(entries))
	failed := 0
	for idx, entry := range entries {
		if err := s.dispatchEvent(entry); err != nil {
			failed++
			responses = append(responses, map[string]any{"ErrorCode": "InternalFailure", "ErrorMessage": err.Error()})
			continue
		}
		responses = append(responses, map[string]any{"EventId": fmt.Sprintf("evt-%d", idx+1)})
	}

	writeJSON(w, http.StatusOK, map[string]any{"FailedEntryCount": failed, "Entries": responses})
}

func (s *server) handleDeleteRule(w http.ResponseWriter, payload map[string]any) {
	name, ok := stringField(payload, "Name")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "Name is required")
		return
	}
	eventBusName, _ := stringField(payload, "EventBusName")
	if eventBusName == "" {
		eventBusName = defaultEventBus
	}

	s.mu.Lock()
	delete(s.rules, ruleKey(eventBusName, name))
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *server) dispatchEvent(entry map[string]any) error {
	eventBusName, _ := stringField(entry, "EventBusName")
	if eventBusName == "" {
		eventBusName = defaultEventBus
	}

	event := normalizeEvent(entry)

	s.mu.RLock()
	rules := make([]Rule, 0)
	for _, rule := range s.rules {
		if rule.EventBusName != eventBusName || strings.EqualFold(rule.State, "DISABLED") {
			continue
		}
		if matchesEvent(rule.EventPattern, event) {
			rules = append(rules, rule)
		}
	}
	s.mu.RUnlock()

	for _, rule := range rules {
		for _, target := range rule.Targets {
			if err := s.deliverToTarget(target, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeEvent(entry map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range entry {
		result[strings.ToLower(k)] = v
	}
	if detailType, ok := result["detailtype"]; ok {
		result["detail-type"] = detailType
	}
	if detailRaw, ok := result["detail"].(string); ok && detailRaw != "" {
		var detail map[string]any
		if err := json.Unmarshal([]byte(detailRaw), &detail); err == nil {
			result["detail"] = detail
		}
	}
	return result
}

func matchesEvent(pattern string, event map[string]any) bool {
	var expected map[string]any
	if err := json.Unmarshal([]byte(pattern), &expected); err != nil {
		return false
	}
	for key, want := range expected {
		eventValue, ok := event[strings.ToLower(key)]
		if !ok {
			return false
		}
		if !patternValueMatches(want, eventValue) {
			return false
		}
	}
	return true
}

func patternValueMatches(expected any, actual any) bool {
	switch want := expected.(type) {
	case []any:
		actualString := fmt.Sprint(actual)
		for _, candidate := range want {
			if fmt.Sprint(candidate) == actualString {
				return true
			}
		}
		return false
	case map[string]any:
		actualMap, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range want {
			if !patternValueMatches(v, actualMap[k]) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprint(expected) == fmt.Sprint(actual)
	}
}

func (s *server) deliverToTarget(target Target, event map[string]any) error {
	payload, _ := json.Marshal(event)
	switch {
	case strings.Contains(target.Arn, ":lambda:"):
		functionName := targetNameFromARN(target.Arn)
		endpoint := fmt.Sprintf("http://localhost:%d/2015-03-31/functions/%s/invocations", s.lambdaPort, functionName)
		resp, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("lambda returned status %d", resp.StatusCode)
		}
	case strings.Contains(target.Arn, ":sqs:"):
		queueName := targetNameFromARN(target.Arn)
		form := url.Values{}
		form.Set("Action", "SendMessage")
		form.Set("QueueName", queueName)
		form.Set("MessageBody", string(payload))
		resp, err := http.PostForm(fmt.Sprintf("http://localhost:%d/", s.sqsPort), form)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("sqs returned status %d", resp.StatusCode)
		}
	}
	return nil
}

func targetNameFromARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func eventBusARN(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:event-bus/%s", region, accountID, name)
}

func ruleARN(eventBusName, name string) string {
	if eventBusName == defaultEventBus {
		return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", region, accountID, name)
	}
	return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s/%s", region, accountID, eventBusName, name)
}

func ruleKey(bus, name string) string {
	return bus + ":" + name
}

func stringField(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	if !ok {
		return "", false
	}
	return str, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"__type":  code,
		"message": message,
	})
}
