# ☁️ CloudDev

**A free, open-source local AWS emulator — the self-hosted alternative to LocalStack.**

CloudDev runs 17 AWS services locally as a single Go binary with zero runtime dependencies. Built for developers who want fast, offline AWS development without cloud costs or vendor lock-in.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22-blue.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.3.0-green.svg)](https://github.com/Jeffrin-dev/CloudDev/releases)

---

## ✨ Features

- **17 AWS services** emulated locally — S3, DynamoDB, Lambda, SQS, SNS, API Gateway, IAM, STS, KMS, CloudFormation, Step Functions, EventBridge, Secrets Manager, CloudWatch Logs, ElastiCache, and Cognito
- **Single binary** — no Docker, no Python, no Java runtime required
- **Zero config** — works out of the box with your existing AWS CLI and SDKs
- **Web dashboard** — real-time service status at `localhost:4580`
- **IaC support** — auto-detects Terraform, CloudFormation, and Kubernetes configs
- **Lambda hot reload** — instant function updates without restarts
- **Data persistence** — state survives restarts via `~/.clouddev/state.json`
- **Cost estimator** — see estimated AWS costs for your local workloads

---

## 🚀 Quick Start

### Install

```bash
# Clone and build
git clone https://github.com/Jeffrin-dev/CloudDev.git
cd CloudDev
go build -o clouddev .
```

### Initialize and Start

```bash
# Create a new project
./clouddev init my-app
cd my-app

# Start all services
../clouddev up
```

### Use with AWS CLI

Point your AWS CLI to CloudDev by using `--endpoint-url`:

```bash
# S3
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-bucket

# DynamoDB
aws --endpoint-url=http://localhost:4569 dynamodb list-tables

# KMS
aws --endpoint-url=http://localhost:4599 kms create-key --description "my-key"
```

---

## 📦 Services & Ports

| Service | Port | Version |
|---|---|---|
| S3 | 4566 | v0.1.0 |
| DynamoDB | 4569 | v0.1.0 |
| Lambda | 4574 | v0.1.0 |
| SQS | 4576 | v0.1.0 |
| API Gateway | 4572 | v0.1.0 |
| Dashboard | 4580 | v0.1.0 |
| SNS | 4575 | v0.2.0 |
| Secrets Manager | 4584 | v0.2.0 |
| CloudWatch Logs | 4586 | v0.2.0 |
| IAM | 4593 | v0.3.0 |
| STS | 4592 | v0.3.0 |
| KMS | 4599 | v0.3.0 |
| CloudFormation | 4581 | v0.3.0 |
| Step Functions | 4585 | v0.3.0 |
| EventBridge | 4587 | v0.3.0 |
| ElastiCache (Redis) | 4598 | v0.3.0 |
| ElastiCache (HTTP) | 4597 | v0.3.0 |
| Cognito | 4596 | v0.3.0 |

---

## 🗂️ Project Structure

```
clouddev/
├── cmd/                    # CLI commands (up, init, status)
├── internal/
│   ├── config/             # Config parser (clouddev.yml)
│   ├── dashboard/          # Web dashboard
│   ├── docker/             # Docker manager
│   ├── iac/                # IaC parser (Terraform/CloudFormation/K8s)
│   ├── persist/            # State persistence
│   ├── costestimator/      # AWS cost estimator
│   └── services/
│       ├── s3/
│       ├── dynamodb/
│       ├── lambda/
│       ├── sqs/
│       ├── sns/
│       ├── apigateway/
│       ├── iam/
│       ├── sts/
│       ├── kms/
│       ├── cloudformation/
│       ├── stepfunctions/
│       ├── eventbridge/
│       ├── secretsmanager/
│       ├── cloudwatchlogs/
│       ├── elasticache/
│       └── cognito/
├── clouddev.yml            # Project config
├── go.mod
└── main.go
```

---

## ⚙️ Configuration

CloudDev uses a `clouddev.yml` file in your project directory:

```yaml
services:
  s3: true
  dynamodb: true
  lambda: true
  sqs: true
  api_gateway: true

ports:
  s3: 4566
  dynamodb: 4569
  lambda: 4574
  sqs: 4576
  api_gateway: 4572

lambda:
  functions_dir: ./functions
  hot_reload: true
```

---

## 🧪 Running Tests

```bash
go test ./...
```

---

## 🛣️ Roadmap

- [ ] ElastiCache Cluster mode
- [ ] Cognito hosted UI
- [ ] SQS FIFO queues
- [ ] Lambda layers support
- [ ] CloudWatch Metrics
- [ ] X-Ray tracing

---

## 🤝 Contributing

We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) to get started.

---

## 📄 License

Apache 2.0 — see [LICENSE](LICENSE) for details.

---

## ⭐ Star History

If CloudDev saves you time or money, please consider giving it a ⭐ on GitHub. It helps others find the project!

> Built with ❤️ using Claude (Instructor) + Codex (Developer)
