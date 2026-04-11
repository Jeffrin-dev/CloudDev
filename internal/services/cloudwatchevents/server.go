package cloudwatchevents

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const xmlContentType = "text/xml"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

type Rule struct {
	Name               string
	ScheduleExpression string
	EventPattern       string
	State              string
	Arn                string
}

type Target struct {
	Id  string
	Arn string
}

type server struct {
	mu      sync.RWMutex
	rules   map[string]Rule
	targets map[string]map[string]Target
}

func newServer() *server {
	return &server{
		rules:   make(map[string]Rule),
		targets: make(map[string]map[string]Target),
	}
}

func Start(port int) error {
	if port == 0 {
		port = 4590
	}
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

	action := r.FormValue("Action")
	switch action {
	case "PutRule":
		s.putRule(w, r)
	case "DeleteRule":
		s.deleteRule(w, r)
	case "ListRules":
		s.listRules(w, r)
	case "DescribeRule":
		s.describeRule(w, r)
	case "PutTargets":
		s.putTargets(w, r)
	case "RemoveTargets":
		s.removeTargets(w, r)
	case "ListTargetsByRule":
		s.listTargetsByRule(w, r)
	case "PutEvents":
		s.putEvents(w, r)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) putRule(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("Name"))
	if name == "" {
		writeError(w, "MissingParameter", "Name is required")
		return
	}

	rule := Rule{
		Name:               name,
		ScheduleExpression: r.FormValue("ScheduleExpression"),
		EventPattern:       r.FormValue("EventPattern"),
		State:              r.FormValue("State"),
		Arn:                ruleARN(name),
	}
	if rule.State == "" {
		rule.State = "ENABLED"
	}

	s.mu.Lock()
	s.rules[name] = rule
	if _, ok := s.targets[name]; !ok {
		s.targets[name] = make(map[string]Target)
	}
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<PutRuleResponse><PutRuleResult><RuleArn>%s</RuleArn></PutRuleResult><ResponseMetadata><RequestId>req-putrule</RequestId></ResponseMetadata></PutRuleResponse>", x(rule.Arn)))
}

func (s *server) deleteRule(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("Name"))
	if name == "" {
		writeError(w, "MissingParameter", "Name is required")
		return
	}

	s.mu.Lock()
	delete(s.rules, name)
	delete(s.targets, name)
	s.mu.Unlock()

	writeXML(w, "<DeleteRuleResponse><ResponseMetadata><RequestId>req-deleterule</RequestId></ResponseMetadata></DeleteRuleResponse>")
}

func (s *server) listRules(w http.ResponseWriter, r *http.Request) {
	prefix := r.FormValue("NamePrefix")

	s.mu.RLock()
	rules := make([]Rule, 0, len(s.rules))
	for _, rule := range s.rules {
		if prefix != "" && !strings.HasPrefix(rule.Name, prefix) {
			continue
		}
		rules = append(rules, rule)
	}
	s.mu.RUnlock()

	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })

	var b strings.Builder
	b.WriteString("<ListRulesResponse><ListRulesResult>")
	for _, rule := range rules {
		b.WriteString("<Rules><member>")
		b.WriteString("<Name>" + x(rule.Name) + "</Name>")
		b.WriteString("<Arn>" + x(rule.Arn) + "</Arn>")
		if rule.ScheduleExpression != "" {
			b.WriteString("<ScheduleExpression>" + x(rule.ScheduleExpression) + "</ScheduleExpression>")
		}
		if rule.EventPattern != "" {
			b.WriteString("<EventPattern>" + x(rule.EventPattern) + "</EventPattern>")
		}
		b.WriteString("<State>" + x(rule.State) + "</State>")
		b.WriteString("</member></Rules>")
	}
	b.WriteString("</ListRulesResult><ResponseMetadata><RequestId>req-listrules</RequestId></ResponseMetadata></ListRulesResponse>")
	writeXML(w, b.String())
}

func (s *server) describeRule(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("Name"))
	if name == "" {
		writeError(w, "MissingParameter", "Name is required")
		return
	}

	s.mu.RLock()
	rule, ok := s.rules[name]
	s.mu.RUnlock()
	if !ok {
		writeError(w, "ResourceNotFoundException", "Rule does not exist")
		return
	}

	writeXML(w, fmt.Sprintf("<DescribeRuleResponse><DescribeRuleResult><Name>%s</Name><Arn>%s</Arn><ScheduleExpression>%s</ScheduleExpression><EventPattern>%s</EventPattern><State>%s</State></DescribeRuleResult><ResponseMetadata><RequestId>req-describerule</RequestId></ResponseMetadata></DescribeRuleResponse>", x(rule.Name), x(rule.Arn), x(rule.ScheduleExpression), x(rule.EventPattern), x(rule.State)))
}

func (s *server) putTargets(w http.ResponseWriter, r *http.Request) {
	ruleName := strings.TrimSpace(r.FormValue("Rule"))
	if ruleName == "" {
		writeError(w, "MissingParameter", "Rule is required")
		return
	}

	s.mu.Lock()
	if _, ok := s.rules[ruleName]; !ok {
		s.mu.Unlock()
		writeError(w, "ResourceNotFoundException", "Rule does not exist")
		return
	}
	if _, ok := s.targets[ruleName]; !ok {
		s.targets[ruleName] = make(map[string]Target)
	}
	for i := 1; ; i++ {
		id := strings.TrimSpace(r.FormValue(fmt.Sprintf("Targets.member.%d.Id", i)))
		arn := strings.TrimSpace(r.FormValue(fmt.Sprintf("Targets.member.%d.Arn", i)))
		if id == "" && arn == "" {
			break
		}
		if id == "" || arn == "" {
			continue
		}
		s.targets[ruleName][id] = Target{Id: id, Arn: arn}
	}
	s.mu.Unlock()

	writeXML(w, "<PutTargetsResponse><PutTargetsResult><FailedEntryCount>0</FailedEntryCount></PutTargetsResult><ResponseMetadata><RequestId>req-puttargets</RequestId></ResponseMetadata></PutTargetsResponse>")
}

func (s *server) removeTargets(w http.ResponseWriter, r *http.Request) {
	ruleName := strings.TrimSpace(r.FormValue("Rule"))
	if ruleName == "" {
		writeError(w, "MissingParameter", "Rule is required")
		return
	}

	s.mu.Lock()
	if _, ok := s.rules[ruleName]; !ok {
		s.mu.Unlock()
		writeError(w, "ResourceNotFoundException", "Rule does not exist")
		return
	}
	for i := 1; ; i++ {
		id := strings.TrimSpace(r.FormValue(fmt.Sprintf("Ids.member.%d", i)))
		if id == "" {
			break
		}
		delete(s.targets[ruleName], id)
	}
	s.mu.Unlock()

	writeXML(w, "<RemoveTargetsResponse><RemoveTargetsResult><FailedEntryCount>0</FailedEntryCount></RemoveTargetsResult><ResponseMetadata><RequestId>req-removetargets</RequestId></ResponseMetadata></RemoveTargetsResponse>")
}

func (s *server) listTargetsByRule(w http.ResponseWriter, r *http.Request) {
	ruleName := strings.TrimSpace(r.FormValue("Rule"))
	if ruleName == "" {
		writeError(w, "MissingParameter", "Rule is required")
		return
	}

	s.mu.RLock()
	if _, ok := s.rules[ruleName]; !ok {
		s.mu.RUnlock()
		writeError(w, "ResourceNotFoundException", "Rule does not exist")
		return
	}
	targetMap := s.targets[ruleName]
	targets := make([]Target, 0, len(targetMap))
	for _, target := range targetMap {
		targets = append(targets, target)
	}
	s.mu.RUnlock()

	sort.Slice(targets, func(i, j int) bool { return targets[i].Id < targets[j].Id })

	var b strings.Builder
	b.WriteString("<ListTargetsByRuleResponse><ListTargetsByRuleResult>")
	for _, target := range targets {
		b.WriteString("<Targets><member><Id>" + x(target.Id) + "</Id><Arn>" + x(target.Arn) + "</Arn></member></Targets>")
	}
	b.WriteString("</ListTargetsByRuleResult><ResponseMetadata><RequestId>req-listtargets</RequestId></ResponseMetadata></ListTargetsByRuleResponse>")
	writeXML(w, b.String())
}

func (s *server) putEvents(w http.ResponseWriter, r *http.Request) {
	entryCount := 0
	for i := 1; ; i++ {
		source := r.FormValue("Entries.member." + strconv.Itoa(i) + ".Source")
		detailType := r.FormValue("Entries.member." + strconv.Itoa(i) + ".DetailType")
		detail := r.FormValue("Entries.member." + strconv.Itoa(i) + ".Detail")
		if source == "" && detailType == "" && detail == "" {
			break
		}
		entryCount++
	}

	var b strings.Builder
	b.WriteString("<PutEventsResponse><PutEventsResult><FailedEntryCount>0</FailedEntryCount>")
	for i := 0; i < entryCount; i++ {
		b.WriteString("<Entries><member><EventId>evt-")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("</EventId></member></Entries>")
	}
	b.WriteString("</PutEventsResult><ResponseMetadata><RequestId>req-putevents</RequestId></ResponseMetadata></PutEventsResponse>")
	writeXML(w, b.String())
}

func ruleARN(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", region, accountID, name)
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", x(code), x(message)))
}

func x(v string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}
