package msk

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

const (
	region      = "us-east-1"
	accountID   = "000000000000"
	contentType = "application/json"
)

type BrokerNodeGroup struct {
	InstanceType  string   `json:"instanceType"`
	ClientSubnets []string `json:"clientSubnets"`
}

type Cluster struct {
	ClusterName         string          `json:"clusterName"`
	ClusterArn          string          `json:"clusterArn"`
	State               string          `json:"state"`
	NumberOfBrokerNodes int             `json:"numberOfBrokerNodes"`
	KafkaVersion        string          `json:"kafkaVersion"`
	BrokerNodeGroupInfo BrokerNodeGroup `json:"brokerNodeGroupInfo"`
}

type createClusterRequest struct {
	ClusterName         string          `json:"clusterName"`
	NumberOfBrokerNodes int             `json:"numberOfBrokerNodes"`
	KafkaVersion        string          `json:"kafkaVersion"`
	BrokerNodeGroupInfo BrokerNodeGroup `json:"brokerNodeGroupInfo"`
}

type server struct {
	mu       sync.RWMutex
	clusters map[string]Cluster
}

func newServer() *server {
	return &server{clusters: make(map[string]Cluster)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case r.Method == http.MethodPost && path == "/v1/clusters":
		s.handleCreateCluster(w, r)
	case r.Method == http.MethodGet && path == "/v1/clusters":
		s.handleListClusters(w)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/clusters/") && strings.HasSuffix(path, "/bootstrap-brokers"):
		clusterArn := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/clusters/"), "/bootstrap-brokers")
		s.handleGetBootstrapBrokers(w, clusterArn)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/v1/clusters/") && strings.HasSuffix(path, "/reboot-broker"):
		clusterArn := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/clusters/"), "/reboot-broker")
		s.handleRebootBroker(w, clusterArn)
	case strings.HasPrefix(path, "/v1/clusters/"):
		clusterArn := strings.TrimPrefix(path, "/v1/clusters/")
		switch r.Method {
		case http.MethodGet:
			s.handleDescribeCluster(w, clusterArn)
		case http.MethodDelete:
			s.handleDeleteCluster(w, clusterArn)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	}
}

func (s *server) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	var req createClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	cluster := Cluster{
		ClusterName:         req.ClusterName,
		ClusterArn:          fmt.Sprintf("arn:aws:kafka:%s:%s:cluster/%s/%s", region, accountID, req.ClusterName, uuidString()),
		State:               "ACTIVE",
		NumberOfBrokerNodes: req.NumberOfBrokerNodes,
		KafkaVersion:        req.KafkaVersion,
		BrokerNodeGroupInfo: req.BrokerNodeGroupInfo,
	}
	s.mu.Lock()
	s.clusters[cluster.ClusterArn] = cluster
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, cluster)
}

func (s *server) handleListClusters(w http.ResponseWriter) {
	s.mu.RLock()
	clusters := make([]Cluster, 0, len(s.clusters))
	for _, c := range s.clusters {
		clusters = append(clusters, c)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"clusterInfoList": clusters,
		"ClusterInfoList": clusters,
	})
}

func (s *server) handleDescribeCluster(w http.ResponseWriter, clusterArn string) {
	s.mu.RLock()
	cluster, ok := s.clusters[clusterArn]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "cluster not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"clusterInfo": cluster,
		"ClusterInfo": cluster,
	})
}

func (s *server) handleDeleteCluster(w http.ResponseWriter, clusterArn string) {
	s.mu.Lock()
	_, ok := s.clusters[clusterArn]
	if ok {
		delete(s.clusters, clusterArn)
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "cluster not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"clusterArn": clusterArn,
		"ClusterArn": clusterArn,
		"state":      "DELETING",
		"State":      "DELETING",
	})
}

func (s *server) handleGetBootstrapBrokers(w http.ResponseWriter, clusterArn string) {
	s.mu.RLock()
	_, ok := s.clusters[clusterArn]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "cluster not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"bootstrapBrokerString":          "localhost:9092,localhost:9093",
		"BootstrapBrokerString":          "localhost:9092,localhost:9093",
		"bootstrapBrokerStringSaslScram": "localhost:9094",
		"BootstrapBrokerStringSaslScram": "localhost:9094",
	})
}

func (s *server) handleRebootBroker(w http.ResponseWriter, clusterArn string) {
	s.mu.RLock()
	_, ok := s.clusters[clusterArn]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "cluster not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"clusterArn": clusterArn,
		"ClusterArn": clusterArn,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func uuidString() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	hexstr := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexstr[0:8], hexstr[8:12], hexstr[12:16], hexstr[16:20], hexstr[20:32])
}
