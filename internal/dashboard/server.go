package dashboard

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

type serviceStatus struct {
	Port    int  `json:"port"`
	Running bool `json:"running"`
}

type statusResponse struct {
	Services map[string]serviceStatus `json:"services"`
}

type server struct {
	mu       sync.RWMutex
	services map[string]int
}

func newServer(services map[string]int) *server {
	copyServices := make(map[string]int, len(services))
	for name, port := range services {
		copyServices[name] = port
	}
	return &server{services: copyServices}
}

func Start(port int, services map[string]int) error {
	srv := newServer(services)
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/status", srv.handleStatus)
	return http.ListenAndServe(net.JoinHostPort("", intToString(port)), mux)
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statusResponse{Services: s.getStatuses()})
}

func (s *server) getStatuses() map[string]serviceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statuses := make(map[string]serviceStatus, len(s.services))
	for name, port := range s.services {
		statuses[name] = serviceStatus{
			Port:    port,
			Running: isPortOpen(port),
		}
	}
	return statuses
}

func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", intToString(port)), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func intToString(v int) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		d := v % 10
		buf = append([]byte{byte('0' + d)}, buf...)
		v /= 10
	}
	if negative {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>CloudDev Dashboard</title>
  <style>
    body { margin: 0; font-family: Arial, sans-serif; background: #f3f4f6; color: #111827; }
    header { background: #111827; color: #ffffff; padding: 20px 24px; }
    header h1 { margin: 0; font-size: 24px; }
    main { max-width: 900px; margin: 24px auto; padding: 0 16px; }
    .card { background: #ffffff; border-radius: 10px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); padding: 16px; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 12px; border-bottom: 1px solid #e5e7eb; }
    th { font-size: 14px; color: #374151; }
    .running { color: #16a34a; font-weight: bold; }
    .stopped { color: #dc2626; font-weight: bold; }
    .muted { color: #6b7280; font-size: 13px; margin-top: 10px; }
  </style>
</head>
<body>
  <header>
    <h1>☁️ CloudDev Dashboard</h1>
  </header>
  <main>
    <div class="card">
      <table>
        <thead>
          <tr>
            <th>Service</th>
            <th>Port</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody id="status-body"></tbody>
      </table>
      <div class="muted">Auto-refresh every 5 seconds</div>
    </div>
  </main>

  <script>
    const labels = {
      s3: 'S3',
      dynamodb: 'DynamoDB',
      lambda: 'Lambda',
      sqs: 'SQS',
      api_gateway: 'API Gateway',
      sns: 'SNS',
      secrets_manager: 'Secrets Manager',
      cloudwatch_logs: 'CloudWatch Logs',
      iam: 'IAM',
      sts: 'STS',
      kms: 'KMS',
      cloudformation: 'CloudFormation',
      step_functions: 'Step Functions',
      eventbridge: 'EventBridge',
      elasticache: 'ElastiCache',
      elasticache_http: 'ElastiCache HTTP',
      cognito: 'Cognito'
    };

    async function refreshStatus() {
      try {
        const res = await fetch('/api/status');
        if (!res.ok) {
          return;
        }
        const data = await res.json();
        const tbody = document.getElementById('status-body');
        tbody.innerHTML = '';

        const order = [
          's3',
          'dynamodb',
          'lambda',
          'sqs',
          'api_gateway',
          'sns',
          'secrets_manager',
          'cloudwatch_logs',
          'iam',
          'sts',
          'kms',
          'cloudformation',
          'step_functions',
          'eventbridge',
          'elasticache',
          'elasticache_http',
          'cognito'
        ];
        order.forEach((key) => {
          const svc = data.services[key];
          if (!svc) return;

          const tr = document.createElement('tr');
          const statusClass = svc.running ? 'running' : 'stopped';
          const statusText = svc.running ? 'Running' : 'Stopped';
          const statusDot = svc.running ? '🟢' : '🔴';
          tr.innerHTML = ` + "`" + `
            <td>${labels[key] || key}</td>
            <td>${svc.port}</td>
            <td class="${statusClass}">${statusDot} ${statusText}</td>
          ` + "`" + `;
          tbody.appendChild(tr);
        });
      } catch (_) {
      }
    }

    refreshStatus();
    setInterval(refreshStatus, 5000);
  </script>
</body>
</html>`
