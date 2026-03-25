package xray

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

const jsonContentType = "application/json"

type Segment struct {
	Id        string  `json:"id"`
	TraceId   string  `json:"trace_id"`
	Name      string  `json:"name"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Fault     bool    `json:"fault"`
	Error     bool    `json:"error"`
}

type server struct {
	mu       sync.RWMutex
	segments map[string][]Segment
	services map[string]struct{}
}

func newServer() *server {
	return &server{
		segments: make(map[string][]Segment),
		services: make(map[string]struct{}),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "Only POST is supported"})
		return
	}

	switch r.URL.Path {
	case "/TraceSegments":
		s.handlePutTraceSegments(w, r)
	case "/TelemetryRecords":
		s.handlePutTelemetryRecords(w)
	case "/TraceSummaries":
		s.handleGetTraceSummaries(w)
	case "/Traces":
		s.handleBatchGetTraces(w, r)
	case "/ServiceGraph":
		s.handleGetServiceGraph(w)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not found"})
	}
}

func (s *server) handlePutTraceSegments(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TraceSegmentDocuments []any `json:"TraceSegmentDocuments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Invalid JSON body"})
		return
	}

	stored := 0
	s.mu.Lock()
	for _, item := range req.TraceSegmentDocuments {
		doc, ok := extractDocument(item)
		if !ok || doc == "" {
			continue
		}
		var seg Segment
		if err := json.Unmarshal([]byte(doc), &seg); err != nil {
			continue
		}
		if seg.TraceId == "" || seg.Id == "" {
			continue
		}
		s.segments[seg.TraceId] = append(s.segments[seg.TraceId], seg)
		if seg.Name != "" {
			s.services[seg.Name] = struct{}{}
		}
		stored++
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"UnprocessedTraceSegments": []any{},
		"StoredSegments":           stored,
	})
}

func extractDocument(item any) (string, bool) {
	switch v := item.(type) {
	case string:
		return v, true
	case map[string]any:
		raw, ok := v["Document"].(string)
		return raw, ok
	default:
		return "", false
	}
}

func (s *server) handlePutTelemetryRecords(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"Accepted": true})
}

func (s *server) handleGetTraceSummaries(w http.ResponseWriter) {
	s.mu.RLock()
	summaries := make([]map[string]any, 0, len(s.segments))
	for traceID, segments := range s.segments {
		if len(segments) == 0 {
			continue
		}
		start := segments[0].StartTime
		end := segments[0].EndTime
		hasFault := false
		hasError := false
		for _, seg := range segments {
			if seg.StartTime < start {
				start = seg.StartTime
			}
			if seg.EndTime > end {
				end = seg.EndTime
			}
			hasFault = hasFault || seg.Fault
			hasError = hasError || seg.Error
		}
		summaries = append(summaries, map[string]any{
			"Id":        traceID,
			"StartTime": start,
			"EndTime":   end,
			"HasFault":  hasFault,
			"HasError":  hasError,
		})
	}
	s.mu.RUnlock()

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i]["Id"].(string) < summaries[j]["Id"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"TraceSummaries": summaries})
}

func (s *server) handleBatchGetTraces(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TraceIds []string `json:"TraceIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Invalid JSON body"})
		return
	}

	s.mu.RLock()
	traces := make([]map[string]any, 0, len(req.TraceIds))
	for _, traceID := range req.TraceIds {
		segments := append([]Segment(nil), s.segments[traceID]...)
		if len(segments) == 0 {
			continue
		}
		traces = append(traces, map[string]any{
			"Id":       traceID,
			"Segments": segments,
		})
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Traces": traces})
}

func (s *server) handleGetServiceGraph(w http.ResponseWriter) {
	s.mu.RLock()
	services := make([]map[string]any, 0, len(s.services))
	for name := range s.services {
		services = append(services, map[string]any{"ReferenceId": 0, "Name": name})
	}
	s.mu.RUnlock()

	sort.Slice(services, func(i, j int) bool {
		return services[i]["Name"].(string) < services[j]["Name"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"Services": services})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
