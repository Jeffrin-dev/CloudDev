package sqs

import (
	"fmt"
	"net/http"
	"sync"
)

const xmlContentType = "text/xml"

type Message struct {
	MessageId     string
	Body          string
	ReceiptHandle string
}

type queue struct {
	name     string
	url      string
	messages []Message
}

type server struct {
	mu            sync.Mutex
	port          int
	queues        map[string]*queue
	nextMessageID int
	nextReceiptID int
}

func newServer(port int) *server {
	return &server{
		port:   port,
		queues: make(map[string]*queue),
	}
}

func Start(port int) error {
	srv := newServer(port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
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
	case "CreateQueue":
		s.createQueue(w, r)
	case "DeleteQueue":
		s.deleteQueue(w, r)
	case "ListQueues":
		s.listQueues(w)
	case "SendMessage":
		s.sendMessage(w, r)
	case "ReceiveMessage":
		s.receiveMessage(w, r)
	case "DeleteMessage":
		s.deleteMessage(w, r)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) createQueue(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("QueueName")
	if name == "" {
		writeError(w, "MissingParameter", "QueueName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	q, exists := s.queues[name]
	if !exists {
		q = &queue{name: name, url: s.queueURL(name)}
		s.queues[name] = q
	}

	writeXML(w, fmt.Sprintf("<CreateQueueResponse><CreateQueueResult><QueueUrl>%s</QueueUrl></CreateQueueResult><ResponseMetadata><RequestId>req-createqueue</RequestId></ResponseMetadata></CreateQueueResponse>", q.url))
}

func (s *server) deleteQueue(w http.ResponseWriter, r *http.Request) {
	q := s.findQueue(r.FormValue("QueueUrl"), r.FormValue("QueueName"))
	if q == nil {
		writeError(w, "AWS.SimpleQueueService.NonExistentQueue", "Queue does not exist")
		return
	}

	s.mu.Lock()
	delete(s.queues, q.name)
	s.mu.Unlock()

	writeXML(w, "<DeleteQueueResponse><ResponseMetadata><RequestId>req-deletequeue</RequestId></ResponseMetadata></DeleteQueueResponse>")
}

func (s *server) listQueues(w http.ResponseWriter) {
	s.mu.Lock()
	result := ""
	for _, q := range s.queues {
		result += fmt.Sprintf("<QueueUrl>%s</QueueUrl>", q.url)
	}
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<ListQueuesResponse><ListQueuesResult>%s</ListQueuesResult><ResponseMetadata><RequestId>req-listqueues</RequestId></ResponseMetadata></ListQueuesResponse>", result))
}

func (s *server) sendMessage(w http.ResponseWriter, r *http.Request) {
	q := s.findQueue(r.FormValue("QueueUrl"), r.FormValue("QueueName"))
	if q == nil {
		writeError(w, "AWS.SimpleQueueService.NonExistentQueue", "Queue does not exist")
		return
	}

	body := r.FormValue("MessageBody")

	s.mu.Lock()
	s.nextMessageID++
	s.nextReceiptID++
	msg := Message{
		MessageId:     fmt.Sprintf("msg-%d", s.nextMessageID),
		Body:          body,
		ReceiptHandle: fmt.Sprintf("rh-%d", s.nextReceiptID),
	}
	q.messages = append(q.messages, msg)
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<SendMessageResponse><SendMessageResult><MD5OfMessageBody>mock-md5</MD5OfMessageBody><MessageId>%s</MessageId></SendMessageResult><ResponseMetadata><RequestId>req-sendmessage</RequestId></ResponseMetadata></SendMessageResponse>", msg.MessageId))
}

func (s *server) receiveMessage(w http.ResponseWriter, r *http.Request) {
	q := s.findQueue(r.FormValue("QueueUrl"), r.FormValue("QueueName"))
	if q == nil {
		writeError(w, "AWS.SimpleQueueService.NonExistentQueue", "Queue does not exist")
		return
	}

	s.mu.Lock()
	var messageXML string
	if len(q.messages) > 0 {
		m := q.messages[0]
		messageXML = fmt.Sprintf("<Message><MessageId>%s</MessageId><ReceiptHandle>%s</ReceiptHandle><Body>%s</Body></Message>", m.MessageId, m.ReceiptHandle, m.Body)
	}
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<ReceiveMessageResponse><ReceiveMessageResult>%s</ReceiveMessageResult><ResponseMetadata><RequestId>req-receivemessage</RequestId></ResponseMetadata></ReceiveMessageResponse>", messageXML))
}

func (s *server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	q := s.findQueue(r.FormValue("QueueUrl"), r.FormValue("QueueName"))
	if q == nil {
		writeError(w, "AWS.SimpleQueueService.NonExistentQueue", "Queue does not exist")
		return
	}

	handle := r.FormValue("ReceiptHandle")
	if handle == "" {
		writeError(w, "MissingParameter", "ReceiptHandle is required")
		return
	}

	s.mu.Lock()
	filtered := make([]Message, 0, len(q.messages))
	for _, m := range q.messages {
		if m.ReceiptHandle != handle {
			filtered = append(filtered, m)
		}
	}
	q.messages = filtered
	s.mu.Unlock()

	writeXML(w, "<DeleteMessageResponse><ResponseMetadata><RequestId>req-deletemessage</RequestId></ResponseMetadata></DeleteMessageResponse>")
}

func (s *server) findQueue(url, name string) *queue {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name != "" {
		return s.queues[name]
	}
	if url == "" {
		return nil
	}
	for _, q := range s.queues {
		if q.url == url {
			return q
		}
	}
	return nil
}

func (s *server) queueURL(name string) string {
	return fmt.Sprintf("http://localhost:%d/queue/%s", s.port, name)
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", code, message))
}
