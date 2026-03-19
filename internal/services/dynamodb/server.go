package dynamodb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/clouddev/clouddev/internal/persist"
)

const jsonContentType = "application/x-amz-json-1.0"

type table struct {
	hashKey string
	items   map[string]map[string]interface{}
}

type server struct {
	mu     sync.RWMutex
	tables map[string]*table
}

var (
	persistedStateMu sync.RWMutex
	persistedState   = persist.DynamoDBState{Tables: make(map[string]persist.DynamoDBTableState)}
)

func newServer() *server {
	return &server{tables: make(map[string]*table)}
}

func Start(port int) error {
	srv := newServer()
	srv.restore(loadPersistedState())
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func LoadState(state persist.DynamoDBState) {
	persistedStateMu.Lock()
	defer persistedStateMu.Unlock()
	persistedState = clonePersistedDynamoState(state)
}

func loadPersistedState() persist.DynamoDBState {
	persistedStateMu.RLock()
	defer persistedStateMu.RUnlock()
	return clonePersistedDynamoState(persistedState)
}

func (s *server) restore(state persist.DynamoDBState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tables = make(map[string]*table, len(state.Tables))
	for tableName, tableState := range state.Tables {
		items := make(map[string]map[string]interface{}, len(tableState.Items))
		for itemKey, item := range tableState.Items {
			items[itemKey] = cloneMap(item)
		}
		s.tables[tableName] = &table{
			hashKey: tableState.HashKey,
			items:   items,
		}
	}
}

func clonePersistedDynamoState(state persist.DynamoDBState) persist.DynamoDBState {
	out := persist.DynamoDBState{Tables: make(map[string]persist.DynamoDBTableState, len(state.Tables))}
	for tableName, tableState := range state.Tables {
		items := make(map[string]map[string]interface{}, len(tableState.Items))
		for itemKey, item := range tableState.Items {
			items[itemKey] = cloneMap(item)
		}
		out.Tables[tableName] = persist.DynamoDBTableState{
			HashKey: tableState.HashKey,
			Items:   items,
		}
	}
	return out
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

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "SerializationException", "Invalid JSON body")
		return
	}

	switch target {
	case "DynamoDB_20120810.CreateTable":
		s.handleCreateTable(w, payload)
	case "DynamoDB_20120810.DescribeTable":
		s.handleDescribeTable(w, payload)
	case "DynamoDB_20120810.DeleteTable":
		s.handleDeleteTable(w, payload)
	case "DynamoDB_20120810.ListTables":
		s.handleListTables(w)
	case "DynamoDB_20120810.PutItem":
		s.handlePutItem(w, payload)
	case "DynamoDB_20120810.GetItem":
		s.handleGetItem(w, payload)
	case "DynamoDB_20120810.DeleteItem":
		s.handleDeleteItem(w, payload)
	case "DynamoDB_20120810.Scan":
		s.handleScan(w, payload)
	case "DynamoDB_20120810.Query":
		s.handleQuery(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateTable(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok || tableName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}

	hashKey, ok := extractHashKey(payload)
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "KeySchema with HASH key is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tables[tableName]; exists {
		writeError(w, http.StatusBadRequest, "ResourceInUseException", "Table already exists")
		return
	}

	s.tables[tableName] = &table{hashKey: hashKey, items: make(map[string]map[string]interface{})}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"TableDescription": map[string]interface{}{
			"TableName":   tableName,
			"TableStatus": "ACTIVE",
			"KeySchema": []map[string]interface{}{{
				"AttributeName": hashKey,
				"KeyType":       "HASH",
			}},
		},
	})
}

func (s *server) handleDeleteTable(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok || tableName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tables[tableName]; !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}
	delete(s.tables, tableName)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"TableDescription": map[string]interface{}{
			"TableName":   tableName,
			"TableStatus": "DELETING",
		},
	})
}

func (s *server) handleDescribeTable(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok || tableName == "" {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}

	s.mu.RLock()
	tbl, exists := s.tables[tableName]
	s.mu.RUnlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Table": map[string]interface{}{
			"TableName":   tableName,
			"TableStatus": "ACTIVE",
			"KeySchema": []map[string]interface{}{{
				"AttributeName": tbl.hashKey,
				"KeyType":       "HASH",
			}},
		},
	})
}

func (s *server) handleListTables(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]string, 0, len(s.tables))
	for n := range s.tables {
		names = append(names, n)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	writeJSON(w, http.StatusOK, map[string]interface{}{"TableNames": names})
}

func (s *server) handlePutItem(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}
	item, ok := objectField(payload, "Item")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Item is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tbl, exists := s.tables[tableName]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	keyValueRaw, ok := item[tbl.hashKey]
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Missing hash key in item")
		return
	}
	itemKey, ok := marshalKey(keyValueRaw)
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Invalid hash key value")
		return
	}

	tbl.items[itemKey] = cloneMap(item)
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleGetItem(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}
	key, ok := objectField(payload, "Key")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Key is required")
		return
	}

	s.mu.RLock()
	tbl, exists := s.tables[tableName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}
	keyRaw, ok := key[tbl.hashKey]
	if !ok {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ValidationException", "Missing hash key in key")
		return
	}
	itemKey, ok := marshalKey(keyRaw)
	if !ok {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ValidationException", "Invalid hash key value")
		return
	}
	item, found := tbl.items[itemKey]
	s.mu.RUnlock()

	if !found {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"Item": cloneMap(item)})
}

func (s *server) handleDeleteItem(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}
	key, ok := objectField(payload, "Key")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Key is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tbl, exists := s.tables[tableName]
	if !exists {
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}
	keyRaw, ok := key[tbl.hashKey]
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Missing hash key in key")
		return
	}
	itemKey, ok := marshalKey(keyRaw)
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "Invalid hash key value")
		return
	}
	delete(tbl.items, itemKey)
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *server) handleScan(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}

	s.mu.RLock()
	tbl, exists := s.tables[tableName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}
	items := make([]map[string]interface{}, 0, len(tbl.items))
	for _, item := range tbl.items {
		items = append(items, cloneMap(item))
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Items":        items,
		"Count":        len(items),
		"ScannedCount": len(items),
	})
}

func (s *server) handleQuery(w http.ResponseWriter, payload map[string]interface{}) {
	tableName, ok := stringField(payload, "TableName")
	if !ok {
		writeError(w, http.StatusBadRequest, "ValidationException", "TableName is required")
		return
	}

	s.mu.RLock()
	tbl, exists := s.tables[tableName]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusBadRequest, "ResourceNotFoundException", "Requested resource not found")
		return
	}

	items := make([]map[string]interface{}, 0)
	if keyConds, ok := payload["KeyConditions"].(map[string]interface{}); ok {
		if condRaw, ok := keyConds[tbl.hashKey].(map[string]interface{}); ok {
			op, _ := condRaw["ComparisonOperator"].(string)
			if strings.EqualFold(op, "EQ") {
				if list, ok := condRaw["AttributeValueList"].([]interface{}); ok && len(list) > 0 {
					if itemKey, ok := marshalKey(list[0]); ok {
						if item, found := tbl.items[itemKey]; found {
							items = append(items, cloneMap(item))
						}
					}
				}
			}
		}
	}
	if len(items) == 0 {
		for _, item := range tbl.items {
			items = append(items, cloneMap(item))
		}
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Items":        items,
		"Count":        len(items),
		"ScannedCount": len(items),
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

func extractHashKey(payload map[string]interface{}) (string, bool) {
	keySchema, ok := payload["KeySchema"].([]interface{})
	if !ok {
		return "", false
	}
	for _, entry := range keySchema {
		item, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := item["AttributeName"].(string)
		keyType, _ := item["KeyType"].(string)
		if name != "" && keyType == "HASH" {
			return name, true
		}
	}
	return "", false
}

func marshalKey(v interface{}) (string, bool) {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return "", false
	}
	return string(b), true
}

func stringField(m map[string]interface{}, name string) (string, bool) {
	v, ok := m[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func objectField(m map[string]interface{}, name string) (map[string]interface{}, bool) {
	v, ok := m[name]
	if !ok {
		return nil, false
	}
	obj, ok := v.(map[string]interface{})
	return obj, ok
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
