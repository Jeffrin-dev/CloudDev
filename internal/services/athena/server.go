package athena

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const jsonContentType = "application/x-amz-json-1.1"

type QueryExecution struct {
	QueryExecutionId   string `json:"QueryExecutionId"`
	Query              string `json:"Query"`
	State              string `json:"State"`
	StateChangeReason  string `json:"StateChangeReason"`
	SubmissionDateTime string `json:"SubmissionDateTime"`
}

type NamedQuery struct {
	NamedQueryId string `json:"NamedQueryId"`
	Name         string `json:"Name"`
	Database     string `json:"Database"`
	Query        string `json:"Query"`
}

type server struct {
	mu              sync.RWMutex
	queryExecutions map[string]QueryExecution
	namedQueries    map[string]NamedQuery
	counter         uint64
}

func newServer() *server {
	return &server{
		queryExecutions: make(map[string]QueryExecution),
		namedQueries:    make(map[string]NamedQuery),
	}
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
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "Missing X-Amz-Target header")
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "Invalid JSON body")
		return
	}

	switch target {
	case "AmazonAthena.StartQueryExecution":
		s.handleStartQueryExecution(w, payload)
	case "AmazonAthena.GetQueryExecution":
		s.handleGetQueryExecution(w, payload)
	case "AmazonAthena.GetQueryResults":
		s.handleGetQueryResults(w, payload)
	case "AmazonAthena.ListQueryExecutions":
		s.handleListQueryExecutions(w)
	case "AmazonAthena.StopQueryExecution":
		s.handleStopQueryExecution(w, payload)
	case "AmazonAthena.CreateNamedQuery":
		s.handleCreateNamedQuery(w, payload)
	case "AmazonAthena.ListNamedQueries":
		s.handleListNamedQueries(w)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleStartQueryExecution(w http.ResponseWriter, payload map[string]interface{}) {
	queryString, _ := stringField(payload, "QueryString")
	queryExecutionID := s.newID("query")
	queryExecution := QueryExecution{
		QueryExecutionId:   queryExecutionID,
		Query:              queryString,
		State:              "SUCCEEDED",
		SubmissionDateTime: time.Now().UTC().Format(time.RFC3339),
	}

	s.mu.Lock()
	s.queryExecutions[queryExecutionID] = queryExecution
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"QueryExecutionId": queryExecutionID,
	})
}

func (s *server) handleGetQueryExecution(w http.ResponseWriter, payload map[string]interface{}) {
	queryExecutionID, ok := stringField(payload, "QueryExecutionId")
	if !ok || queryExecutionID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "QueryExecutionId is required")
		return
	}

	s.mu.RLock()
	queryExecution, exists := s.queryExecutions[queryExecutionID]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "QueryExecutionId not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"QueryExecution": map[string]interface{}{
			"QueryExecutionId": queryExecution.QueryExecutionId,
			"Query":            queryExecution.Query,
			"Status": map[string]interface{}{
				"State":              queryExecution.State,
				"StateChangeReason":  queryExecution.StateChangeReason,
				"SubmissionDateTime": queryExecution.SubmissionDateTime,
			},
		},
	})
}

func (s *server) handleGetQueryResults(w http.ResponseWriter, payload map[string]interface{}) {
	queryExecutionID, ok := stringField(payload, "QueryExecutionId")
	if !ok || queryExecutionID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "QueryExecutionId is required")
		return
	}

	s.mu.RLock()
	_, exists := s.queryExecutions[queryExecutionID]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "QueryExecutionId not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ResultSet": map[string]interface{}{
			"ResultSetMetadata": map[string]interface{}{
				"ColumnInfo": []map[string]interface{}{{"Name": "result", "Type": "varchar"}},
			},
			"Rows": []map[string]interface{}{{"Data": []map[string]interface{}{{"VarCharValue": "mock-result"}}}},
		},
	})
}

func (s *server) handleListQueryExecutions(w http.ResponseWriter) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.queryExecutions))
	for id := range s.queryExecutions {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)
	writeJSON(w, http.StatusOK, map[string]interface{}{"QueryExecutionIds": ids})
}

func (s *server) handleStopQueryExecution(w http.ResponseWriter, payload map[string]interface{}) {
	queryExecutionID, ok := stringField(payload, "QueryExecutionId")
	if !ok || queryExecutionID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "QueryExecutionId is required")
		return
	}

	s.mu.Lock()
	queryExecution, exists := s.queryExecutions[queryExecutionID]
	if exists {
		queryExecution.State = "CANCELLED"
		s.queryExecutions[queryExecutionID] = queryExecution
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "InvalidRequestException", "QueryExecutionId not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"QueryExecutionId": queryExecutionID})
}

func (s *server) handleCreateNamedQuery(w http.ResponseWriter, payload map[string]interface{}) {
	name, _ := stringField(payload, "Name")
	query, _ := stringField(payload, "QueryString")
	database := ""
	if ctx, ok := payload["Database"]; ok {
		database, _ = ctx.(string)
	}
	id := s.newID("named")
	entry := NamedQuery{NamedQueryId: id, Name: name, Database: database, Query: query}

	s.mu.Lock()
	s.namedQueries[id] = entry
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"NamedQueryId": id})
}

func (s *server) handleListNamedQueries(w http.ResponseWriter) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.namedQueries))
	for id := range s.namedQueries {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)
	writeJSON(w, http.StatusOK, map[string]interface{}{"NamedQueryIds": ids})
}

func (s *server) newID(prefix string) string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

func stringField(payload map[string]interface{}, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{"__type": code, "message": message})
}
