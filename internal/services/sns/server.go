package sns

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const xmlContentType = "text/xml"

const (
	accountID = "000000000000"
	region    = "us-east-1"
)

type topic struct {
	arn  string
	name string
}

type subscription struct {
	arn      string
	topicArn string
	protocol string
	endpoint string
}

type server struct {
	mu                 sync.Mutex
	sqsPort            int
	topics             map[string]*topic
	subscriptions      map[string]*subscription
	nextSubscriptionID int
}

func newServer(sqsPort int) *server {
	return &server{
		sqsPort:       sqsPort,
		topics:        make(map[string]*topic),
		subscriptions: make(map[string]*subscription),
	}
}

func Start(port int, sqsPort int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer(sqsPort))
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

	switch r.FormValue("Action") {
	case "CreateTopic":
		s.createTopic(w, r)
	case "DeleteTopic":
		s.deleteTopic(w, r)
	case "ListTopics":
		s.listTopics(w)
	case "Subscribe":
		s.subscribe(w, r)
	case "Publish":
		s.publish(w, r)
	case "ListSubscriptions":
		s.listSubscriptions(w)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) createTopic(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("Name")
	if name == "" {
		writeError(w, "MissingParameter", "Name is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	arn := topicARN(name)
	if _, ok := s.topics[arn]; !ok {
		s.topics[arn] = &topic{arn: arn, name: name}
	}

	writeXML(w, fmt.Sprintf("<CreateTopicResponse><CreateTopicResult><TopicArn>%s</TopicArn></CreateTopicResult><ResponseMetadata><RequestId>req-createtopic</RequestId></ResponseMetadata></CreateTopicResponse>", arn))
}

func (s *server) deleteTopic(w http.ResponseWriter, r *http.Request) {
	topicArn := r.FormValue("TopicArn")
	if topicArn == "" {
		writeError(w, "MissingParameter", "TopicArn is required")
		return
	}

	s.mu.Lock()
	delete(s.topics, topicArn)
	for arn, sub := range s.subscriptions {
		if sub.topicArn == topicArn {
			delete(s.subscriptions, arn)
		}
	}
	s.mu.Unlock()

	writeXML(w, "<DeleteTopicResponse><ResponseMetadata><RequestId>req-deletetopic</RequestId></ResponseMetadata></DeleteTopicResponse>")
}

func (s *server) listTopics(w http.ResponseWriter) {
	s.mu.Lock()
	arns := make([]string, 0, len(s.topics))
	for arn := range s.topics {
		arns = append(arns, arn)
	}
	s.mu.Unlock()
	sort.Strings(arns)

	items := ""
	for _, arn := range arns {
		items += fmt.Sprintf("<member><TopicArn>%s</TopicArn></member>", arn)
	}

	writeXML(w, fmt.Sprintf("<ListTopicsResponse><ListTopicsResult><Topics>%s</Topics></ListTopicsResult><ResponseMetadata><RequestId>req-listtopics</RequestId></ResponseMetadata></ListTopicsResponse>", items))
}

func (s *server) subscribe(w http.ResponseWriter, r *http.Request) {
	topicArn := r.FormValue("TopicArn")
	protocol := r.FormValue("Protocol")
	endpoint := r.FormValue("Endpoint")
	if topicArn == "" || protocol == "" || endpoint == "" {
		writeError(w, "MissingParameter", "TopicArn, Protocol, and Endpoint are required")
		return
	}

	s.mu.Lock()
	if _, ok := s.topics[topicArn]; !ok {
		s.mu.Unlock()
		writeError(w, "NotFound", "Topic does not exist")
		return
	}
	s.nextSubscriptionID++
	subArn := topicArn + ":" + strconv.Itoa(s.nextSubscriptionID)
	s.subscriptions[subArn] = &subscription{arn: subArn, topicArn: topicArn, protocol: protocol, endpoint: endpoint}
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<SubscribeResponse><SubscribeResult><SubscriptionArn>%s</SubscriptionArn></SubscribeResult><ResponseMetadata><RequestId>req-subscribe</RequestId></ResponseMetadata></SubscribeResponse>", subArn))
}

func (s *server) publish(w http.ResponseWriter, r *http.Request) {
	topicArn := r.FormValue("TopicArn")
	message := r.FormValue("Message")
	if topicArn == "" || message == "" {
		writeError(w, "MissingParameter", "TopicArn and Message are required")
		return
	}

	s.mu.Lock()
	if _, ok := s.topics[topicArn]; !ok {
		s.mu.Unlock()
		writeError(w, "NotFound", "Topic does not exist")
		return
	}
	subs := make([]subscription, 0)
	for _, sub := range s.subscriptions {
		if sub.topicArn == topicArn {
			subs = append(subs, *sub)
		}
	}
	s.mu.Unlock()

	for _, sub := range subs {
		if strings.EqualFold(sub.protocol, "sqs") {
			if err := s.deliverToSQS(sub.endpoint, message); err != nil {
				writeError(w, "InternalError", err.Error())
				return
			}
		}
	}

	writeXML(w, "<PublishResponse><PublishResult><MessageId>msg-publish</MessageId></PublishResult><ResponseMetadata><RequestId>req-publish</RequestId></ResponseMetadata></PublishResponse>")
}

func (s *server) listSubscriptions(w http.ResponseWriter) {
	s.mu.Lock()
	subs := make([]subscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		subs = append(subs, *sub)
	}
	s.mu.Unlock()
	sort.Slice(subs, func(i, j int) bool { return subs[i].arn < subs[j].arn })

	items := ""
	for _, sub := range subs {
		items += fmt.Sprintf("<member><SubscriptionArn>%s</SubscriptionArn><TopicArn>%s</TopicArn><Protocol>%s</Protocol><Endpoint>%s</Endpoint></member>", sub.arn, sub.topicArn, sub.protocol, sub.endpoint)
	}

	writeXML(w, fmt.Sprintf("<ListSubscriptionsResponse><ListSubscriptionsResult><Subscriptions>%s</Subscriptions></ListSubscriptionsResult><ResponseMetadata><RequestId>req-listsubscriptions</RequestId></ResponseMetadata></ListSubscriptionsResponse>", items))
}

func (s *server) deliverToSQS(queueURL string, message string) error {
	form := url.Values{}
	form.Set("Action", "SendMessage")
	form.Set("QueueUrl", queueURL)
	form.Set("MessageBody", message)

	resp, err := http.PostForm(fmt.Sprintf("http://localhost:%d/", s.sqsPort), form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sqs returned status %d", resp.StatusCode)
	}
	return nil
}

func topicARN(name string) string {
	return fmt.Sprintf("arn:aws:sns:%s:%s:%s", region, accountID, name)
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", code, message))
}
