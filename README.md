# ☁️ CloudDev
 
**A free, open-source local AWS emulator — the self-hosted alternative to LocalStack.**
 
CloudDev runs 23 AWS services locally as a single Go binary with zero runtime dependencies. Built for developers who want fast, offline AWS development without cloud costs or vendor lock-in.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22-blue.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.4.0-green.svg)](https://github.com/Jeffrin-dev/CloudDev/releases)

---

## ✨ Features
 
- **23 AWS services** emulated locally — S3, DynamoDB, Lambda, SQS (+ FIFO), SNS, API Gateway, IAM, STS, KMS, CloudFormation, Step Functions, EventBridge, Secrets Manager, CloudWatch Logs, CloudWatch Metrics, ElastiCache, Cognito, Lambda Layers, X-Ray, Route53, SSM Parameter Store, and Rekognition
- **Single binary** — no Docker, no Python, no Java runtime required
- **Zero config** — works out of the box with your existing AWS CLI and SDKs
- **Web dashboard** — real-time service status cards at `localhost:4580`
- **IaC support** — auto-detects Terraform, CloudFormation, and Kubernetes configs
- **Lambda hot reload** — instant function updates without restarts
- **Data persistence** — state survives restarts via `~/.clouddev/state.json`
- **Cost estimator** — see estimated AWS costs for your local workloads
 
---
 
## 🚀 Quick Start
 
### Install
 
```bash
git clone https://github.com/Jeffrin-dev/CloudDev.git
cd CloudDev
go build -o clouddev .
```
 
### Initialize and Start
 
```bash
./clouddev init my-app
cd my-app
../clouddev up
```
 
### Use with AWS CLI
 
```bash
# S3
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-bucket
 
# DynamoDB
aws --endpoint-url=http://localhost:4569 dynamodb list-tables
 
# KMS
aws --endpoint-url=http://localhost:4599 kms create-key --description "my-key"
 
# SSM Parameter Store
aws --endpoint-url=http://localhost:4583 ssm put-parameter \
    --name "/myapp/db/host" --value "localhost" --type String
 
# CloudWatch Metrics
aws --endpoint-url=http://localhost:4582 cloudwatch put-metric-data \
    --namespace "MyApp" --metric-name "RequestCount" --value 42 --unit Count
 
# Route53
aws --endpoint-url=http://localhost:4589 route53 create-hosted-zone \
    --name example.com --caller-reference ref-1
 
# X-Ray
aws xray put-trace-segments \
    --endpoint-url http://localhost:4588 \
    --trace-segment-documents '{"id":"seg-1","trace_id":"1-abc-123","name":"my-service"}'
 
# Lambda Layers
aws --endpoint-url=http://localhost:4578 lambda publish-layer-version \
    --layer-name my-layer --description "My layer" \
    --compatible-runtimes python3.9 go1.x \
    --content S3Bucket=my-bucket,S3Key=my-layer.zip
 
# Rekognition
aws --endpoint-url=http://localhost:4594 rekognition detect-labels \
    --image '{"S3Object":{"Bucket":"my-bucket","Name":"photo.jpg"}}'
```
 
---
 
## 📦 Services & Ports
 
| Service | Port | Version |
|---|---|---|
| S3 | 4566 | v0.1.0 |
| DynamoDB | 4569 | v0.1.0 |
| Lambda | 4574 | v0.1.0 |
| SQS (+ FIFO) | 4576 | v0.1.0 / v0.4.0 |
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
| CloudWatch Metrics | 4582 | v0.4.0 |
| Lambda Layers | 4578 | v0.4.0 |
| X-Ray | 4588 | v0.4.0 |
| Route53 | 4589 | v0.4.0 |
| SSM Parameter Store | 4583 | v0.4.0 |
| Rekognition | 4594 | v0.4.0 |
 
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
│       ├── lambdalayers/
│       ├── sqs/
│       ├── sns/
│       ├── apigateway/
│       ├── iam/
│       ├── sts/
│       ├── kms/
│       ├── cloudformation/
│       ├── cloudwatchlogs/
│       ├── cloudwatchmetrics/
│       ├── stepfunctions/
│       ├── eventbridge/
│       ├── secretsmanager/
│       ├── elasticache/
│       ├── cognito/
│       ├── xray/
│       ├── route53/
│       ├── ssm/
│       └── rekognition/
├── clouddev.yml
├── go.mod
└── main.go
```
 
---
 
## ⚙️ Configuration
 
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
 
- [ ] CloudWatch Events
- [ ] SES email emulation
- [ ] DynamoDB Streams
- [ ] Lambda function URLs
- [ ] API Gateway v2 (HTTP API)
- [ ] Bedrock (mock LLM responses)
 
---
 
## 🤝 Contributing
 
We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) to get started.
 
---
 
## 📄 License
 
Apache 2.0 — see [LICENSE](LICENSE) for details.
 
---
 
## ⭐ Star History
 
If CloudDev saves you time or money, please consider giving it a ⭐ on GitHub!

> Built for devs
