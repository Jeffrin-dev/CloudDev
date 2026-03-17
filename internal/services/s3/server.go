package s3

import (
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const xmlContentType = "application/xml"

type object struct {
	data         []byte
	lastModified time.Time
	etag         string
}

type server struct {
	mu      sync.RWMutex
	buckets map[string]map[string]object
}

func newServer() *server {
	return &server{buckets: make(map[string]map[string]object)}
}

func Start(port int) error {
	srv := newServer()
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket, key, ok := parsePath(r.URL.EscapedPath())
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidURI", "Could not parse request URI")
		return
	}

	if bucket == "" {
		if r.Method == http.MethodGet {
			s.listBuckets(w)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
		return
	}

	if key == "" {
		s.handleBucket(w, r, bucket)
		return
	}

	s.handleObject(w, r, bucket, key)
}

func (s *server) handleBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	switch r.Method {
	case http.MethodPut:
		s.createBucket(w, bucket)
	case http.MethodDelete:
		s.deleteBucket(w, bucket)
	case http.MethodGet:
		s.listObjects(w, bucket)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

func (s *server) handleObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	switch r.Method {
	case http.MethodPut:
		s.putObject(w, r, bucket, key)
	case http.MethodGet:
		s.getObject(w, bucket, key)
	case http.MethodDelete:
		s.deleteObject(w, bucket, key)
	case http.MethodHead:
		s.headObject(w, bucket, key)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

func (s *server) createBucket(w http.ResponseWriter, bucket string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.buckets[bucket]; !exists {
		s.buckets[bucket] = make(map[string]object)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) deleteBucket(w http.ResponseWriter, bucket string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	objs, exists := s.buckets[bucket]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}
	if len(objs) > 0 {
		writeError(w, http.StatusConflict, "BucketNotEmpty", "The bucket you tried to delete is not empty")
		return
	}

	delete(s.buckets, bucket)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listBuckets(w http.ResponseWriter) {
	type bucketXML struct {
		Name         string `xml:"Name"`
		CreationDate string `xml:"CreationDate"`
	}
	type response struct {
		XMLName xml.Name `xml:"ListAllMyBucketsResult"`
		Xmlns   string   `xml:"xmlns,attr"`
		Owner   struct {
			ID          string `xml:"ID"`
			DisplayName string `xml:"DisplayName"`
		} `xml:"Owner"`
		Buckets struct {
			Bucket []bucketXML `xml:"Bucket"`
		} `xml:"Buckets"`
	}

	s.mu.RLock()
	names := make([]string, 0, len(s.buckets))
	for name := range s.buckets {
		names = append(names, name)
	}
	s.mu.RUnlock()
	sort.Strings(names)

	resp := response{Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/"}
	resp.Owner.ID = "clouddev"
	resp.Owner.DisplayName = "clouddev"
	now := time.Now().UTC().Format(time.RFC3339)
	for _, name := range names {
		resp.Buckets.Bucket = append(resp.Buckets.Bucket, bucketXML{Name: name, CreationDate: now})
	}
	writeXML(w, http.StatusOK, resp)
}

func (s *server) listObjects(w http.ResponseWriter, bucket string) {
	type content struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
		Size         int    `xml:"Size"`
		StorageClass string `xml:"StorageClass"`
	}
	type response struct {
		XMLName     xml.Name  `xml:"ListBucketResult"`
		Xmlns       string    `xml:"xmlns,attr"`
		Name        string    `xml:"Name"`
		Prefix      string    `xml:"Prefix"`
		KeyCount    int       `xml:"KeyCount"`
		MaxKeys     int       `xml:"MaxKeys"`
		IsTruncated bool      `xml:"IsTruncated"`
		Contents    []content `xml:"Contents"`
	}

	s.mu.RLock()
	objs, exists := s.buckets[bucket]
	if !exists {
		s.mu.RUnlock()
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}

	keys := make([]string, 0, len(objs))
	for key := range objs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	resp := response{
		Xmlns:       "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:        bucket,
		Prefix:      "",
		MaxKeys:     1000,
		IsTruncated: false,
	}
	for _, key := range keys {
		obj := objs[key]
		resp.Contents = append(resp.Contents, content{
			Key:          key,
			LastModified: obj.lastModified.UTC().Format(time.RFC3339),
			ETag:         fmt.Sprintf("\"%s\"", obj.etag),
			Size:         len(obj.data),
			StorageClass: "STANDARD",
		})
	}
	s.mu.RUnlock()

	resp.KeyCount = len(resp.Contents)
	writeXML(w, http.StatusOK, resp)
}

func (s *server) putObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "Could not read request body")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	objs, exists := s.buckets[bucket]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}

	etag := fmt.Sprintf("%x", md5.Sum(body))
	objs[key] = object{data: body, lastModified: time.Now().UTC(), etag: etag}
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", etag))
	w.WriteHeader(http.StatusOK)
}

func (s *server) getObject(w http.ResponseWriter, bucket, key string) {
	obj, ok := s.getStoredObject(bucket, key)
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist")
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj.data)))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", obj.etag))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(obj.data)
}

func (s *server) headObject(w http.ResponseWriter, bucket, key string) {
	obj, ok := s.getStoredObject(bucket, key)
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist")
		return
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj.data)))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", obj.etag))
	w.WriteHeader(http.StatusOK)
}

func (s *server) getStoredObject(bucket, key string) (object, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	objs, exists := s.buckets[bucket]
	if !exists {
		return object{}, false
	}
	obj, exists := objs[key]
	if !exists {
		return object{}, false
	}
	return obj, true
}

func (s *server) deleteObject(w http.ResponseWriter, bucket, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	objs, exists := s.buckets[bucket]
	if !exists {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}
	delete(objs, key)
	w.WriteHeader(http.StatusNoContent)
}

func writeXML(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	type errorResponse struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}
	writeXML(w, status, errorResponse{Code: code, Message: message})
}

func parsePath(escapedPath string) (bucket, key string, ok bool) {
	trimmed := strings.TrimPrefix(escapedPath, "/")
	if trimmed == "" {
		return "", "", true
	}

	parts := strings.SplitN(trimmed, "/", 2)
	bucket, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", false
	}

	if len(parts) == 1 {
		return bucket, "", true
	}

	key, err = url.PathUnescape(parts[1])
	if err != nil {
		return "", "", false
	}
	return bucket, key, true
}
