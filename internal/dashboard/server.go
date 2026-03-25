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
    .status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; }
    .service-card { background: #ffffff; border-radius: 10px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); padding: 14px; border: 1px solid #e5e7eb; }
    .service-name { font-weight: 700; margin-bottom: 8px; }
    .service-meta { font-size: 14px; color: #374151; margin-bottom: 6px; }
    .service-status { display: flex; align-items: center; gap: 8px; font-weight: 700; }
    .status-dot { width: 10px; height: 10px; border-radius: 9999px; display: inline-block; }
    .running .status-dot { background: #16a34a; }
    .stopped .status-dot { background: #dc2626; }
    .running .status-text { color: #16a34a; }
    .stopped .status-text { color: #dc2626; }
    .muted { color: #6b7280; font-size: 13px; margin-top: 10px; }
  </style>
</head>
<body>
  <header>
    <h1>☁️ CloudDev Dashboard</h1>
  </header>
  <main>
    <div id="status-grid" class="status-grid"></div>
    <div class="muted">Auto-refresh every 5 seconds</div>
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
      cloudwatch_metrics: 'CloudWatch Metrics',
      iam: 'IAM',
      sts: 'STS',
      kms: 'KMS',
      cloudformation: 'CloudFormation',
      step_functions: 'Step Functions',
      eventbridge: 'EventBridge',
      xray: 'X-Ray',
      route53: 'Route53',
      elasticache: 'ElastiCache',
      elasticache_http: 'ElastiCache HTTP',
      cognito: 'Cognito',
      lambda_layers: 'Lambda Layers',
      ssm: 'SSM Parameter Store',
      rekognition: 'Rekognition'
    };

    async function refreshStatus() {
      try {
        const res = await fetch('/api/status');
        if (!res.ok) {
          return;
        }
        const data = await res.json();
        const grid = document.getElementById('status-grid');
        grid.innerHTML = '';

        const order = [
          's3',
          'dynamodb',
          'lambda',
          'sqs',
          'api_gateway',
          'sns',
          'secrets_manager',
          'ssm',
          'cloudwatch_logs',
          'cloudwatch_metrics',
          'iam',
          'sts',
          'kms',
          'cloudformation',
          'step_functions',
          'eventbridge',
          'xray',
          'route53',
          'elasticache',
          'elasticache_http',
          'cognito',
          'lambda_layers',
          'rekognition'
        ];
        order.forEach((key) => {
          const svc = data.services[key];
          if (!svc) return;

          const card = document.createElement('div');
          card.className = 'service-card';
          const statusClass = svc.running ? 'running' : 'stopped';
          const statusText = svc.running ? 'Running' : 'Stopped';
          card.innerHTML = ` + "`" + `
            <div class="service-name">${labels[key] || key}</div>
            <div class="service-meta">Port: ${svc.port}</div>
            <div class="service-status ${statusClass}">
              <span class="status-dot"></span>
              <span class="status-text">${statusText}</span>
            </div>
          ` + "`" + `;
          grid.appendChild(card);
        });
      } catch (_) {
      }
    }

    refreshStatus();
    setInterval(refreshStatus, 5000);
  </script>
</body>
</html>`
