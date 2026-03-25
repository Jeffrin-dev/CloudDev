package lambdalayers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const layersBasePath = "/2018-11-14/layers"

const (
	region    = "us-east-1"
	accountID = "000000000000"
)

type Layer struct {
	LayerName          string
	LayerArn           string
	Version            int
	Description        string
	CompatibleRuntimes []string
}

type publishLayerVersionRequest struct {
	Description        string   `json:"Description"`
	CompatibleRuntimes []string `json:"CompatibleRuntimes"`
}

type server struct {
	mu     sync.RWMutex
	layers map[string][]Layer
}

func newServer() *server {
	return &server{layers: make(map[string][]Layer)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == layersBasePath {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
			return
		}
		s.listLayers(w)
		return
	}

	if !strings.HasPrefix(r.URL.Path, layersBasePath+"/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	remainder := strings.TrimPrefix(r.URL.Path, layersBasePath+"/")
	parts := strings.Split(remainder, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] != "versions" {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	layerName := parts[0]

	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		s.publishLayerVersion(w, r, layerName)
	case len(parts) == 2 && r.Method == http.MethodGet:
		s.listLayerVersions(w, layerName)
	case len(parts) == 3 && r.Method == http.MethodGet:
		version, err := strconv.Atoi(parts[2])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid version"})
			return
		}
		s.getLayerVersion(w, layerName, version)
	case len(parts) == 3 && r.Method == http.MethodDelete:
		version, err := strconv.Atoi(parts[2])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid version"})
			return
		}
		s.deleteLayerVersion(w, layerName, version)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *server) publishLayerVersion(w http.ResponseWriter, r *http.Request, layerName string) {
	var req publishLayerVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}

	s.mu.Lock()
	version := len(s.layers[layerName]) + 1
	layer := Layer{
		LayerName:          layerName,
		LayerArn:           layerARN(layerName, version),
		Version:            version,
		Description:        req.Description,
		CompatibleRuntimes: req.CompatibleRuntimes,
	}
	s.layers[layerName] = append(s.layers[layerName], layer)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"LayerArn": layer.LayerArn,
		"Version":  layer.Version,
	})
}

func (s *server) listLayers(w http.ResponseWriter) {
	s.mu.RLock()
	names := make([]string, 0, len(s.layers))
	for name := range s.layers {
		names = append(names, name)
	}
	sort.Strings(names)

	layers := make([]Layer, 0, len(names))
	for _, name := range names {
		versions := s.layers[name]
		if len(versions) == 0 {
			continue
		}
		layers = append(layers, versions[len(versions)-1])
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"Layers": layers})
}

func (s *server) listLayerVersions(w http.ResponseWriter, layerName string) {
	s.mu.RLock()
	versions, ok := s.layers[layerName]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Layer not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"LayerVersions": versions})
}

func (s *server) getLayerVersion(w http.ResponseWriter, layerName string, version int) {
	s.mu.RLock()
	versions, ok := s.layers[layerName]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Layer not found"})
		return
	}
	for _, layer := range versions {
		if layer.Version == version {
			writeJSON(w, http.StatusOK, layer)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"message": "Layer version not found"})
}

func (s *server) deleteLayerVersion(w http.ResponseWriter, layerName string, version int) {
	s.mu.Lock()
	versions, ok := s.layers[layerName]
	if !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Layer not found"})
		return
	}

	updated := make([]Layer, 0, len(versions))
	deleted := false
	for _, layer := range versions {
		if layer.Version == version {
			deleted = true
			continue
		}
		updated = append(updated, layer)
	}

	if !deleted {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Layer version not found"})
		return
	}

	if len(updated) == 0 {
		delete(s.layers, layerName)
	} else {
		s.layers[layerName] = updated
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"message": "Layer version deleted"})
}

func layerARN(name string, version int) string {
	return fmt.Sprintf("arn:aws:lambda:%s:%s:layer:%s:%d", region, accountID, name, version)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
