package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type server struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
}

type bucket struct {
	created time.Time
	objects map[string][]byte
}

type listAllMyBucketsResult struct {
	XMLName xml.Name    `xml:"ListAllMyBucketsResult"`
	Xmlns   string      `xml:"xmlns,attr"`
	Buckets bucketsList `xml:"Buckets"`
}

type bucketsList struct {
	Bucket []bucketEntry `xml:"Bucket"`
}

type bucketEntry struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type listBucketResult struct {
	XMLName     xml.Name      `xml:"ListBucketResult"`
	Xmlns       string        `xml:"xmlns,attr"`
	Name        string        `xml:"Name"`
	Prefix      string        `xml:"Prefix"`
	Marker      string        `xml:"Marker"`
	MaxKeys     int           `xml:"MaxKeys"`
	IsTruncated bool          `xml:"IsTruncated"`
	Contents    []objectEntry `xml:"Contents,omitempty"`
}

type objectEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	Size         int    `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type errorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

func Start(port int) error {
	s := &server{buckets: make(map[string]*bucket)}
	mux := http.NewServeMux()
	mux.Handle("/", s)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		if r.Method == http.MethodGet {
			s.listBuckets(w)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Unsupported method")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	bucketName := parts[0]
	if bucketName == "" {
		writeError(w, http.StatusBadRequest, "InvalidBucketName", "Bucket name is required")
		return
	}

	if len(parts) == 1 {
		s.handleBucket(w, r, bucketName)
		return
	}

	key := parts[1]
	if key == "" {
		writeError(w, http.StatusBadRequest, "InvalidKey", "Object key is required")
		return
	}

	s.handleObject(w, r, bucketName, key)
}

func (s *server) handleBucket(w http.ResponseWriter, r *http.Request, bucketName string) {
	switch r.Method {
	case http.MethodPut:
		s.createBucket(w, bucketName)
	case http.MethodDelete:
		s.deleteBucket(w, bucketName)
	case http.MethodGet:
		s.listObjects(w, bucketName)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Unsupported method")
	}
}

func (s *server) handleObject(w http.ResponseWriter, r *http.Request, bucketName, key string) {
	switch r.Method {
	case http.MethodPut:
		s.putObject(w, r, bucketName, key)
	case http.MethodGet:
		s.getObject(w, bucketName, key)
	case http.MethodDelete:
		s.deleteObject(w, bucketName, key)
	case http.MethodHead:
		s.headObject(w, bucketName, key)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Unsupported method")
	}
}

func (s *server) createBucket(w http.ResponseWriter, bucketName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.buckets[bucketName]; exists {
		writeError(w, http.StatusConflict, "BucketAlreadyOwnedByYou", "Bucket already exists")
		return
	}

	s.buckets[bucketName] = &bucket{created: time.Now().UTC(), objects: make(map[string][]byte)}
	w.WriteHeader(http.StatusOK)
}

func (s *server) deleteBucket(w http.ResponseWriter, bucketName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, exists := s.buckets[bucketName]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}
	if len(b.objects) > 0 {
		writeError(w, http.StatusConflict, "BucketNotEmpty", "The bucket you tried to delete is not empty")
		return
	}

	delete(s.buckets, bucketName)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listBuckets(w http.ResponseWriter) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.buckets))
	for name := range s.buckets {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]bucketEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, bucketEntry{Name: name, CreationDate: s.buckets[name].created.Format(time.RFC3339)})
	}

	writeXML(w, http.StatusOK, listAllMyBucketsResult{Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/", Buckets: bucketsList{Bucket: entries}})
}

func (s *server) listObjects(w http.ResponseWriter, bucketName string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, exists := s.buckets[bucketName]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}

	keys := make([]string, 0, len(b.objects))
	for key := range b.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	contents := make([]objectEntry, 0, len(keys))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, key := range keys {
		contents = append(contents, objectEntry{Key: key, LastModified: now, Size: len(b.objects[key]), StorageClass: "STANDARD"})
	}

	writeXML(w, http.StatusOK, listBucketResult{Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/", Name: bucketName, Prefix: "", Marker: "", MaxKeys: 1000, IsTruncated: false, Contents: contents})
}

func (s *server) putObject(w http.ResponseWriter, r *http.Request, bucketName, key string) {
	data := new(bytes.Buffer)
	if _, err := data.ReadFrom(r.Body); err != nil {
		writeError(w, http.StatusInternalServerError, "InternalError", "Failed to read request body")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, exists := s.buckets[bucketName]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}

	b.objects[key] = append([]byte(nil), data.Bytes()...)
	w.WriteHeader(http.StatusOK)
}

func (s *server) getObject(w http.ResponseWriter, bucketName, key string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, exists := s.buckets[bucketName]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}

	obj, exists := b.objects[key]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(obj)
}

func (s *server) deleteObject(w http.ResponseWriter, bucketName, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, exists := s.buckets[bucketName]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}

	delete(b.objects, key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) headObject(w http.ResponseWriter, bucketName, key string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, exists := s.buckets[bucketName]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}
	obj, exists := b.objects[key]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist")
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj)))
	w.WriteHeader(http.StatusOK)
}

func writeXML(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeXML(w, status, errorResponse{Code: code, Message: message})
}
