# ☁️ CloudDev — Run AWS Locally

![Go](https://img.shields.io/badge/Go-1.22-blue?logo=go)
![License](https://img.shields.io/badge/License-Apache%202.0-green)
![Status](https://img.shields.io/badge/Status-Active-brightgreen)
![LocalStack Alternative](https://img.shields.io/badge/LocalStack-Alternative-orange)

CloudDev is a lightweight, **100% open-source** developer tool that lets you run core AWS services locally for development and testing — no account required, no paywalls, no surprises.

> A true open-source alternative to LocalStack.

---

## Why CloudDev?

Developing on AWS is slow and expensive:

- Deploy to AWS just to test → wait minutes for provisioning
- Accidental usage of EC2/RDS → unexpected charges
- Event-driven integrations are hard to replicate locally

CloudDev solves this with a single command:
```bash
clouddev up
```

---

## Features

- 🪣 **Amazon S3** — local object storage (port 4566)
- ⚡ **AWS Lambda** — serverless functions with hot reload (port 4574)
- 🗄️ **Amazon DynamoDB** — local NoSQL database (port 4569)
- 📨 **Amazon SQS** — local message queues (port 4576)
- 🌐 **Amazon API Gateway** — local HTTP API routing (port 4572)
- 🖥️ **Web Dashboard** — live service status at http://localhost:4580
- 🔍 **IaC Detection** — auto-configure from Terraform/CloudFormation
- 🔧 **AWS CLI compatible** — use `--endpoint-url` just like real AWS
- ✈️ **Offline-first** — works without internet

---

## Quick Start

### 1. Initialize a project
```bash
clouddev init my-app
cd my-app
```

### 2. Start your local cloud
```bash
clouddev up
```
```
S3 server starting on port 4566
DynamoDB server starting on port 4569
Lambda server starting on port 4574
SQS server starting on port 4576
API Gateway starting on port 4572
Dashboard running at http://localhost:4580
CloudDev is running. Press Ctrl+C to stop...
```

### 3. Use with AWS CLI
```bash
# S3
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-bucket

# DynamoDB
aws --endpoint-url=http://localhost:4569 dynamodb list-tables

# Lambda
aws --endpoint-url=http://localhost:4574 lambda list-functions

# SQS
aws --endpoint-url=http://localhost:4576 sqs list-queues
```

### 4. Auto-detect from Terraform
```bash
clouddev detect
```

Scans your `.tf` files and automatically enables the right services in `clouddev.yml`.

### 5. View Dashboard

Open `http://localhost:4580` in your browser to see live service status.

---

## Configuration (clouddev.yml)
```yaml
services:
  s3: true
  dynamodb: true
  lambda: true
  sqs: false
  api_gateway: true

ports:
  s3: 4566
  lambda: 4574
  dynamodb: 4569
  sqs: 4576
  api_gateway: 4572

lambda:
  hot_reload: true
  functions_dir: ./functions
```

---

## Architecture

| Component | Description |
|---|---|
| CLI Tool | cobra-based CLI — init, up, down, test, detect |
| Config Parser | Reads and validates clouddev.yml |
| Docker Manager | Manages container lifecycle |
| Service Emulation | Go HTTP servers mimicking AWS APIs |
| Web Dashboard | Live browser UI showing service status |
| IaC Parser | Reads Terraform and CloudFormation definitions |

---

## Why not LocalStack?

LocalStack recently ended its free Community Edition — requiring an account and moving to a paid model. CloudDev is built to be the free, open alternative forever.

| Feature | CloudDev | LocalStack Free |
|---|---|---|
| Requires account | ❌ No | ✅ Yes |
| Open source | ✅ Yes | ⚠️ Partial |
| S3, DynamoDB, Lambda | ✅ Yes | ✅ Yes |
| Web dashboard | ✅ Yes | ❌ No |
| IaC auto-detection | ✅ Yes | ❌ No |
| Cost | 🆓 Free forever | 💰 Paid |

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a PR.

---

## License

Apache 2.0 — free to use, forever.
