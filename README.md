# ☁️ CloudDev

**A free, open-source local AWS emulator — the self-hosted alternative to LocalStack.**

CloudDev runs 37 AWS services locally as a single Go binary with zero runtime dependencies. Built for developers who want fast, offline AWS development without cloud costs or vendor lock-in.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22-blue.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.6.0-green.svg)](https://github.com/Jeffrin-dev/CloudDev/releases)

---

## ✨ Features

- **37 AWS services** emulated locally — S3, DynamoDB, DynamoDB Streams, Lambda, Lambda Layers, Lambda Function URLs, SQS (+ FIFO), SNS, API Gateway, API Gateway v2, IAM, STS, KMS, CloudFormation, Step Functions, EventBridge, CloudWatch Events, Secrets Manager, CloudWatch Logs, CloudWatch Metrics, ElastiCache, Cognito, X-Ray, Route53, SSM Parameter Store, Rekognition, SES, Bedrock, Kinesis Data Streams, Kinesis Firehose, ECS, ECR, CloudFront, AppSync, Athena, and ACM
- **Single binary** — no Docker, no Python, no Java runtime required
- **Zero config** — works out of the box with your existing AWS CLI and SDKs
- **Web dashboard** — real-time service status at `localhost:4580` with offline detection
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

# DynamoDB Streams
aws --endpoint-url=http://localhost:4570 dynamodbstreams list-streams

# Lambda
aws --endpoint-url=http://localhost:4574 lambda list-functions

# Lambda Function URLs
curl -X POST http://localhost:4595/2021-10-31/functions/MyFunc/url \
    -H "Content-Type: application/json" \
    -d '{"AuthType":"NONE"}'

# SQS
aws --endpoint-url=http://localhost:4576 sqs create-queue --queue-name my-queue

# SNS
aws --endpoint-url=http://localhost:4575 sns create-topic --name my-topic

# API Gateway v2
curl -X POST http://localhost:4573/v2/apis \
    -H "Content-Type: application/json" \
    -d '{"Name":"MyAPI","ProtocolType":"HTTP"}'

# KMS
aws --endpoint-url=http://localhost:4599 kms create-key --description "my-key"

# SSM Parameter Store
aws --endpoint-url=http://localhost:4583 ssm put-parameter \
    --name "/myapp/db/host" --value "localhost" --type String

# CloudWatch Metrics
aws --endpoint-url=http://localhost:4582 cloudwatch put-metric-data \
    --namespace "MyApp" --metric-name "RequestCount" --value 42 --unit Count

# CloudWatch Events
aws --endpoint-url=http://localhost:4590 events put-rule \
    --name MyRule --schedule-expression "rate(5 minutes)" --state ENABLED

# SES
aws --endpoint-url=http://localhost:4579 ses verify-email-identity \
    --email-address test@example.com

# Route53
aws --endpoint-url=http://localhost:4589 route53 create-hosted-zone \
    --name example.com --caller-reference ref-1

# X-Ray
aws xray put-trace-segments \
    --endpoint-url http://localhost:4588 \
    --trace-segment-documents '{"id":"seg-1","trace_id":"1-abc-123","name":"my-service"}'

# Rekognition
aws --endpoint-url=http://localhost:4594 rekognition detect-labels \
    --image '{"S3Object":{"Bucket":"my-bucket","Name":"photo.jpg"}}'

# Bedrock
curl -X POST http://localhost:4591/foundation-models
curl -X POST http://localhost:4591/model/anthropic.claude-3-sonnet-20240229-v1:0/invoke \
    -H "Content-Type: application/json" \
    -d '{"prompt":"Hello!","max_tokens_to_sample":100}'

# Kinesis Data Streams
aws --endpoint-url=http://localhost:4568 kinesis create-stream \
    --stream-name my-stream --shard-count 2
aws --endpoint-url=http://localhost:4568 kinesis put-record \
    --stream-name my-stream --data "hello" --partition-key "pk1"

# Kinesis Firehose
aws --endpoint-url=http://localhost:4571 firehose create-delivery-stream \
    --delivery-stream-name my-stream --delivery-stream-type DirectPut

# ECS
aws --endpoint-url=http://localhost:4561 ecs create-cluster --cluster-name my-cluster
aws --endpoint-url=http://localhost:4561 ecs register-task-definition \
    --family my-task \
    --container-definitions '[{"name":"app","image":"nginx","memory":128,"cpu":64}]'

# ECR
aws --endpoint-url=http://localhost:4562 ecr create-repository --repository-name my-repo
aws --endpoint-url=http://localhost:4562 ecr get-authorization-token

# CloudFront
curl -X POST http://localhost:4563/2020-05-31/distribution \
    -H "Content-Type: application/xml" \
    -d '<DistributionConfig><Comment>my-cdn</Comment><Origins><Items><Origin><Id>o1</Id><DomainName>s3.amazonaws.com</DomainName></Origin></Items></Origins></DistributionConfig>'

# AppSync
curl -X POST http://localhost:4567/v1/apis \
    -H "Content-Type: application/json" \
    -d '{"name":"my-api","authenticationType":"API_KEY"}'

# Athena
aws --endpoint-url=http://localhost:4564 athena start-query-execution \
    --query-string "SELECT * FROM my_table" \
    --query-execution-context Database=mydb

# ACM
aws --endpoint-url=http://localhost:4560 acm request-certificate \
    --domain-name example.com
```

---

## 📦 Services & Ports

| Service | Port | Since |
|---|---|---|
| ACM | 4560 | v0.6.0 |
| ECS | 4561 | v0.6.0 |
| ECR | 4562 | v0.6.0 |
| CloudFront | 4563 | v0.6.0 |
| Athena | 4564 | v0.6.0 |
| S3 | 4566 | v0.1.0 |
| AppSync | 4567 | v0.6.0 |
| Kinesis Data Streams | 4568 | v0.6.0 |
| DynamoDB | 4569 | v0.1.0 |
| DynamoDB Streams | 4570 | v0.5.0 |
| Kinesis Firehose | 4571 | v0.6.0 |
| API Gateway | 4572 | v0.1.0 |
| API Gateway v2 | 4573 | v0.5.0 |
| Lambda | 4574 | v0.1.0 |
| SNS | 4575 | v0.2.0 |
| SQS (+ FIFO) | 4576 | v0.1.0 |
| Lambda Layers | 4578 | v0.4.0 |
| SES | 4579 | v0.5.0 |
| Dashboard | 4580 | v0.1.0 |
| CloudFormation | 4581 | v0.3.0 |
| CloudWatch Metrics | 4582 | v0.4.0 |
| SSM Parameter Store | 4583 | v0.4.0 |
| Secrets Manager | 4584 | v0.2.0 |
| Step Functions | 4585 | v0.3.0 |
| CloudWatch Logs | 4586 | v0.2.0 |
| EventBridge | 4587 | v0.3.0 |
| X-Ray | 4588 | v0.4.0 |
| Route53 | 4589 | v0.4.0 |
| CloudWatch Events | 4590 | v0.5.0 |
| Bedrock | 4591 | v0.5.0 |
| STS | 4592 | v0.3.0 |
| IAM | 4593 | v0.3.0 |
| Rekognition | 4594 | v0.4.0 |
| Lambda Function URLs | 4595 | v0.5.0 |
| Cognito | 4596 | v0.3.0 |
| ElastiCache (HTTP) | 4597 | v0.3.0 |
| ElastiCache (Redis) | 4598 | v0.3.0 |
| KMS | 4599 | v0.3.0 |

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
│       ├── acm/
│       ├── appsync/
│       ├── athena/
│       ├── apigateway/
│       ├── apigatewayv2/
│       ├── bedrock/
│       ├── cloudformation/
│       ├── cloudfront/
│       ├── cloudwatchevents/
│       ├── cloudwatchlogs/
│       ├── cloudwatchmetrics/
│       ├── cognito/
│       ├── dynamodb/
│       ├── dynamodbstreams/
│       ├── ecr/
│       ├── ecs/
│       ├── elasticache/
│       ├── eventbridge/
│       ├── firehose/
│       ├── iam/
│       ├── kinesis/
│       ├── kms/
│       ├── lambda/
│       ├── lambdalayers/
│       ├── lambdaurls/
│       ├── rekognition/
│       ├── route53/
│       ├── s3/
│       ├── secretsmanager/
│       ├── ses/
│       ├── sns/
│       ├── sqs/
│       ├── ssm/
│       ├── stepfunctions/
│       ├── sts/
│       └── xray/
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

### ✅ v0.1.0
- [x] S3, DynamoDB, Lambda, SQS, API Gateway, Dashboard, IaC Parser

### ✅ v0.2.0
- [x] SNS, Secrets Manager, CloudWatch Logs, Lambda Hot Reload, Data Persistence

### ✅ v0.3.0
- [x] IAM, STS, KMS, CloudFormation, Step Functions, EventBridge, ElastiCache, Cognito

### ✅ v0.4.0
- [x] CloudWatch Metrics, SQS FIFO, Lambda Layers, X-Ray, Route53, SSM Parameter Store, Rekognition

### ✅ v0.5.0
- [x] CloudWatch Events, SES, DynamoDB Streams, Lambda Function URLs, API Gateway v2, Bedrock
- [x] Dashboard offline detection — flips to red when server stops
- [x] DynamoDB UpdateItem support
- [x] DynamoDB → Streams integration

### ✅ v0.6.0
- [x] Kinesis Data Streams
- [x] Kinesis Firehose
- [x] ECS (Elastic Container Service)
- [x] ECR (Elastic Container Registry)
- [x] CloudFront
- [x] AppSync
- [x] Athena
- [x] ACM (Certificate Manager)

---

## 🤝 Contributing

We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) to get started.

---

## 📄 License

Apache 2.0 — see [LICENSE](LICENSE) for details.

---

## ⭐ Star History

If CloudDev saves you time or money, please consider giving it a ⭐ on GitHub!

> Built with Claude (Instructor) · Codex (Developer)
