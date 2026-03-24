package elasticache

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const xmlContentType = "text/xml"

type CacheCluster struct {
	CacheClusterId     string `json:"CacheClusterId"`
	CacheClusterStatus string `json:"CacheClusterStatus"`
	Engine             string `json:"Engine"`
	EngineVersion      string `json:"EngineVersion"`
	NumCacheNodes      int    `json:"NumCacheNodes"`
}

type server struct {
	mu       sync.RWMutex
	kv       map[string]string
	ttl      map[string]time.Time
	clusters map[string]CacheCluster
}

func newServer() *server {
	return &server{
		kv:       make(map[string]string),
		ttl:      make(map[string]time.Time),
		clusters: make(map[string]CacheCluster),
	}
}

func Start(redisPort int, httpPort int) error {
	srv := newServer()

	redisLn, err := net.Listen("tcp", fmt.Sprintf(":%d", redisPort))
	if err != nil {
		return err
	}

	httpSrv := &http.Server{Addr: fmt.Sprintf(":%d", httpPort), Handler: srv}

	errCh := make(chan error, 2)
	go func() {
		errCh <- srv.serveRedis(redisLn)
	}()
	go func() {
		errCh <- httpSrv.ListenAndServe()
	}()

	return <-errCh
}

func (s *server) serveRedis(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.handleRedisConn(conn)
	}
}

func (s *server) handleRedisConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		args, err := readRESPArray(reader)
		if err != nil {
			if err == io.EOF {
				return
			}
			_, _ = writer.WriteString("-ERR invalid request\r\n")
			_ = writer.Flush()
			return
		}
		if len(args) == 0 {
			_, _ = writer.WriteString("-ERR empty command\r\n")
			_ = writer.Flush()
			continue
		}

		cmd := strings.ToUpper(args[0])
		if cmd == "QUIT" {
			_, _ = writer.WriteString("+OK\r\n")
			_ = writer.Flush()
			return
		}

		resp := s.execRedisCommand(cmd, args[1:])
		_, _ = writer.WriteString(resp)
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

func (s *server) execRedisCommand(cmd string, args []string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.evictExpiredLocked()

	switch cmd {
	case "PING":
		return "+PONG\r\n"
	case "SET":
		if len(args) != 2 {
			return "-ERR wrong number of arguments for 'set' command\r\n"
		}
		s.kv[args[0]] = args[1]
		delete(s.ttl, args[0])
		return "+OK\r\n"
	case "GET":
		if len(args) != 1 {
			return "-ERR wrong number of arguments for 'get' command\r\n"
		}
		val, ok := s.kv[args[0]]
		if !ok {
			return "$-1\r\n"
		}
		return fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)
	case "DEL":
		if len(args) != 1 {
			return "-ERR wrong number of arguments for 'del' command\r\n"
		}
		_, existed := s.kv[args[0]]
		delete(s.kv, args[0])
		delete(s.ttl, args[0])
		if existed {
			return ":1\r\n"
		}
		return ":0\r\n"
	case "EXISTS":
		if len(args) != 1 {
			return "-ERR wrong number of arguments for 'exists' command\r\n"
		}
		if _, ok := s.kv[args[0]]; ok {
			return ":1\r\n"
		}
		return ":0\r\n"
	case "EXPIRE":
		if len(args) != 2 {
			return "-ERR wrong number of arguments for 'expire' command\r\n"
		}
		if _, ok := s.kv[args[0]]; !ok {
			return ":0\r\n"
		}
		seconds, err := strconv.Atoi(args[1])
		if err != nil {
			return "-ERR value is not an integer or out of range\r\n"
		}
		s.ttl[args[0]] = time.Now().Add(time.Duration(seconds) * time.Second)
		return ":1\r\n"
	case "TTL":
		if len(args) != 1 {
			return "-ERR wrong number of arguments for 'ttl' command\r\n"
		}
		if _, ok := s.kv[args[0]]; !ok {
			return ":-2\r\n"
		}
		exp, ok := s.ttl[args[0]]
		if !ok {
			return ":-1\r\n"
		}
		remaining := int(time.Until(exp).Seconds())
		if remaining < 0 {
			delete(s.kv, args[0])
			delete(s.ttl, args[0])
			return ":-2\r\n"
		}
		return fmt.Sprintf(":%d\r\n", remaining)
	case "KEYS":
		if len(args) != 1 {
			return "-ERR wrong number of arguments for 'keys' command\r\n"
		}
		matches := make([]string, 0)
		for key := range s.kv {
			if matchesPattern(args[0], key) {
				matches = append(matches, key)
			}
		}
		sort.Strings(matches)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("*%d\r\n", len(matches)))
		for _, key := range matches {
			b.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(key), key))
		}
		return b.String()
	case "FLUSHALL":
		s.kv = make(map[string]string)
		s.ttl = make(map[string]time.Time)
		return "+OK\r\n"
	default:
		return "-ERR unknown command\r\n"
	}
}

func (s *server) evictExpiredLocked() {
	now := time.Now()
	for key, exp := range s.ttl {
		if !exp.After(now) {
			delete(s.ttl, key)
			delete(s.kv, key)
		}
	}
}

func readRESPArray(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" {
		return nil, fmt.Errorf("empty request")
	}
	if line[0] != '*' {
		return nil, fmt.Errorf("expected array")
	}
	count, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}

	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		head, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		head = strings.TrimSuffix(strings.TrimSuffix(head, "\n"), "\r")
		if len(head) == 0 || head[0] != '$' {
			return nil, fmt.Errorf("expected bulk string")
		}
		length, err := strconv.Atoi(head[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		parts = append(parts, string(buf[:length]))
	}
	return parts, nil
}

func matchesPattern(pattern, key string) bool {
	if pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return key == pattern
	}

	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(key[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") {
		last := parts[len(parts)-1]
		return strings.HasSuffix(key, last)
	}
	return true
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
	case "CreateCacheCluster":
		s.createCacheCluster(w, r)
	case "DeleteCacheCluster":
		s.deleteCacheCluster(w, r)
	case "DescribeCacheClusters":
		s.describeCacheClusters(w)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) createCacheCluster(w http.ResponseWriter, r *http.Request) {
	cacheClusterID := r.FormValue("CacheClusterId")
	if cacheClusterID == "" {
		writeError(w, "MissingParameter", "CacheClusterId is required")
		return
	}

	engine := r.FormValue("Engine")
	if engine == "" {
		engine = "redis"
	}

	numCacheNodes := 1
	numCacheNodesValue := r.FormValue("NumCacheNodes")
	if numCacheNodesValue != "" {
		parsed, err := strconv.Atoi(numCacheNodesValue)
		if err != nil || parsed <= 0 {
			writeError(w, "InvalidParameterValue", "NumCacheNodes must be a positive integer")
			return
		}
		numCacheNodes = parsed
	}
	cluster := CacheCluster{
		CacheClusterId:     cacheClusterID,
		CacheClusterStatus: "available",
		Engine:             engine,
		NumCacheNodes:      numCacheNodes,
	}

	s.mu.Lock()
	s.clusters[cluster.CacheClusterId] = cluster
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<CreateCacheClusterResponse><CreateCacheClusterResult><CacheCluster><CacheClusterId>%s</CacheClusterId><CacheClusterStatus>%s</CacheClusterStatus><Engine>%s</Engine><NumCacheNodes>%d</NumCacheNodes></CacheCluster></CreateCacheClusterResult></CreateCacheClusterResponse>", cluster.CacheClusterId, cluster.CacheClusterStatus, cluster.Engine, cluster.NumCacheNodes))
}

func (s *server) deleteCacheCluster(w http.ResponseWriter, r *http.Request) {
	cacheClusterID := r.FormValue("CacheClusterId")
	if cacheClusterID == "" {
		writeError(w, "MissingParameter", "CacheClusterId is required")
		return
	}

	s.mu.Lock()
	_, exists := s.clusters[cacheClusterID]
	if exists {
		delete(s.clusters, cacheClusterID)
	}
	s.mu.Unlock()

	if !exists {
		writeError(w, "CacheClusterNotFound", "CacheCluster not found")
		return
	}

	writeXML(w, fmt.Sprintf("<DeleteCacheClusterResponse><DeleteCacheClusterResult><CacheCluster><CacheClusterId>%s</CacheClusterId><CacheClusterStatus>deleted</CacheClusterStatus></CacheCluster></DeleteCacheClusterResult></DeleteCacheClusterResponse>", cacheClusterID))
}

func (s *server) describeCacheClusters(w http.ResponseWriter) {
	s.mu.RLock()
	clusters := make([]CacheCluster, 0, len(s.clusters))
	for _, cluster := range s.clusters {
		clusters = append(clusters, cluster)
	}
	s.mu.RUnlock()

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].CacheClusterId < clusters[j].CacheClusterId
	})

	result := ""
	for _, cluster := range clusters {
		result += fmt.Sprintf("<CacheCluster><CacheClusterId>%s</CacheClusterId><CacheClusterStatus>%s</CacheClusterStatus><Engine>%s</Engine><NumCacheNodes>%d</NumCacheNodes></CacheCluster>", cluster.CacheClusterId, cluster.CacheClusterStatus, cluster.Engine, cluster.NumCacheNodes)
	}
	writeXML(w, fmt.Sprintf("<DescribeCacheClustersResponse><DescribeCacheClustersResult><CacheClusters>%s</CacheClusters></DescribeCacheClustersResult></DescribeCacheClustersResponse>", result))
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", code, message))
}
