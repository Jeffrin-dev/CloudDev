package ecr

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

const jsonContentType = "application/x-amz-json-1.1"

type Repository struct {
	RepositoryName string `json:"repositoryName"`
	RepositoryArn  string `json:"repositoryArn"`
	RepositoryURI  string `json:"repositoryUri"`
	CreatedAt      string `json:"createdAt"`
}

type Image struct {
	ImageDigest string `json:"imageDigest"`
	ImageTag    string `json:"imageTag"`
}

type storedImage struct {
	Image
	ImageManifest string
}

type server struct {
	mu           sync.RWMutex
	repositories map[string]Repository
	images       map[string][]storedImage
}

func newServer() *server {
	return &server{repositories: map[string]Repository{}, images: map[string][]storedImage{}}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowedException", "Only POST is supported")
		return
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Invalid JSON body")
		return
	}

	switch r.Header.Get("X-Amz-Target") {
	case "AmazonEC2ContainerRegistry_V20150921.CreateRepository":
		s.handleCreateRepository(w, payload)
	case "AmazonEC2ContainerRegistry_V20150921.DeleteRepository":
		s.handleDeleteRepository(w, payload)
	case "AmazonEC2ContainerRegistry_V20150921.DescribeRepositories":
		s.handleDescribeRepositories(w)
	case "AmazonEC2ContainerRegistry_V20150921.ListImages":
		s.handleListImages(w, payload)
	case "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken":
		s.handleGetAuthorizationToken(w)
	case "AmazonEC2ContainerRegistry_V20150921.PutImage":
		s.handlePutImage(w, payload)
	case "AmazonEC2ContainerRegistry_V20150921.BatchGetImage":
		s.handleBatchGetImage(w, payload)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func (s *server) handleCreateRepository(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "repositoryName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "repositoryName is required")
		return
	}
	repo := Repository{RepositoryName: name, RepositoryArn: fmt.Sprintf("arn:aws:ecr:us-east-1:000000000000:repository/%s", name), RepositoryURI: fmt.Sprintf("localhost:4562/%s", name), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.mu.Lock()
	s.repositories[name] = repo
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"repository": repo})
}

func (s *server) handleDeleteRepository(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "repositoryName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "repositoryName is required")
		return
	}
	s.mu.Lock()
	repo, exists := s.repositories[name]
	if exists {
		delete(s.repositories, name)
		delete(s.images, name)
	}
	s.mu.Unlock()
	if !exists {
		writeError(w, http.StatusBadRequest, "RepositoryNotFoundException", "Repository not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"repository": repo})
}

func (s *server) handleDescribeRepositories(w http.ResponseWriter) {
	s.mu.RLock()
	repos := make([]Repository, 0, len(s.repositories))
	for _, repo := range s.repositories {
		repos = append(repos, repo)
	}
	s.mu.RUnlock()
	sort.Slice(repos, func(i, j int) bool { return repos[i].RepositoryName < repos[j].RepositoryName })
	writeJSON(w, http.StatusOK, map[string]interface{}{"repositories": repos})
}

func (s *server) handleListImages(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "repositoryName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "repositoryName is required")
		return
	}
	s.mu.RLock()
	stored := s.images[name]
	s.mu.RUnlock()
	images := make([]Image, 0, len(stored))
	for _, img := range stored {
		images = append(images, img.Image)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"imageIds": images})
}

func (s *server) handleGetAuthorizationToken(w http.ResponseWriter) {
	token := base64.StdEncoding.EncodeToString([]byte("AWS:mock-token"))
	writeJSON(w, http.StatusOK, map[string]interface{}{"authorizationData": []map[string]interface{}{{"authorizationToken": token, "proxyEndpoint": "https://localhost:4562", "expiresAt": time.Now().UTC().Add(12 * time.Hour).Format(time.RFC3339)}}})
}

func (s *server) handlePutImage(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "repositoryName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "repositoryName is required")
		return
	}
	manifest, ok := stringField(payload, "imageManifest")
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "imageManifest is required")
		return
	}
	tag, ok := stringField(payload, "imageTag")
	if !ok {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "imageTag is required")
		return
	}
	s.mu.Lock()
	if _, exists := s.repositories[name]; !exists {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "RepositoryNotFoundException", "Repository not found")
		return
	}
	img := storedImage{Image: Image{ImageDigest: manifestDigest(manifest), ImageTag: tag}, ImageManifest: manifest}
	s.images[name] = append(s.images[name], img)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"image": img.Image})
}

func (s *server) handleBatchGetImage(w http.ResponseWriter, payload map[string]interface{}) {
	name, ok := stringField(payload, "repositoryName")
	if !ok || name == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "repositoryName is required")
		return
	}

	reqImageIDs, _ := payload["imageIds"].([]interface{})
	wanted := map[string]bool{}
	for _, raw := range reqImageIDs {
		imageID, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if tag, ok := imageID["imageTag"].(string); ok && tag != "" {
			wanted[tag] = true
		}
		if digest, ok := imageID["imageDigest"].(string); ok && digest != "" {
			wanted[digest] = true
		}
	}

	s.mu.RLock()
	stored := s.images[name]
	s.mu.RUnlock()
	images := make([]map[string]interface{}, 0)
	for _, img := range stored {
		if len(wanted) == 0 || wanted[img.ImageTag] || wanted[img.ImageDigest] {
			images = append(images, map[string]interface{}{"imageId": img.Image, "imageManifest": img.ImageManifest})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"images": images})
}

func manifestDigest(manifest string) string {
	sum := md5.Sum([]byte(manifest))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stringField(payload map[string]interface{}, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{"__type": code, "message": message})
}
