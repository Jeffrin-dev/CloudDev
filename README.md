# ☁️ CloudDev — Run AWS Locally

![Go](https://img.shields.io/badge/Go-1.22-blue?logo=go)
![License](https://img.shields.io/badge/License-Apache%202.0-green)
![Version](https://img.shields.io/badge/Version-0.2.0-brightgreen)
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
- 🗄️ **Amazon DynamoDB** — local NoSQL database (port 4569)
- ⚡ **AWS Lambda** — serverless functions with hot reload (port 4574)
- 📨 **Amazon SQS** — local message queues (port 4576)
- 🌐 **Amazon API Gateway** — local HTTP routing to Lambda (port 4572)
- 📣 **Amazon SNS** — pub/sub with SQS delivery (port 4575)
- 🔐 **Secrets Manager** — local secret storage (port 4584)
- 📊 **CloudWatch Logs** — local log groups and streams (port 4586)
- 🖥️ **Web Dashboard** — live service status at http://localhost:4580
- 💾 **Data Persistence** — S3 and DynamoDB survive restarts
- 💰 **Cost Estimator** — see how much you save vs real AWS
- 🔍 **IaC Detection** — auto-configure from Terraform, CloudFormation, Kubernetes
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
SNS server starting on port 4575
Secrets Manager starting on port 4584
CloudWatch Logs starting on port 4586
Dashboard running at http://localhost:4580
CloudDev is running. Press Ctrl+C to stop...
```

### 3. Check service status
```bash
clouddev status
```
```
Service      Port   Status
S3           4566   Running
DynamoDB     4569   Running
Lambda       4574   Running
SQS          4576   Running
API Gateway  4572   Running
```

### 4. Estimate AWS costs
```bash
clouddev estimate
```
```
CloudDev Cost Estimator
=======================
Service         Estimated Monthly Cost
S3              $5.00
DynamoDB        $10.00
Lambda          $3.00
-----------------------
Total           $18.00
💰 Running locally with CloudDev saves you ~$18.00/month!
```

### 5. Auto-detect from IaC
```bash
clouddev detect
```

Scans your `.tf`, CloudFormation, and Kubernetes YAML files and automatically enables the right services in `clouddev.yml`.

---

## AWS CLI Examples
```bash
# S3
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-bucket
aws --endpoint-url=http://localhost:4566 s3 cp file.txt s3://my-bucket/

# DynamoDB
aws --endpoint-url=http://localhost:4569 dynamodb create-table \
  --table-name Users \
  --attribute-definitions AttributeName=id,AttributeType=S \
  --key-schema AttributeName=id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST

# Lambda
aws --endpoint-url=http://localhost:4574 lambda create-function \
  --function-name hello \
  --runtime python3.9 \
  --handler hello.handler \
  --role arn:aws:iam::000000000000:role/test \
  --zip-file fileb://hello.zip

# SQS
aws --endpoint-url=http://localhost:4576 sqs create-queue \
  --queue-name my-queue

# SNS
aws --endpoint-url=http://localhost:4575 sns create-topic \
  --name my-topic

# Secrets Manager
aws --endpoint-url=http://localhost:4584 secretsmanager create-secret \
  --name my-secret \
  --secret-string '{"password":"clouddev123"}'

# CloudWatch Logs
aws --endpoint-url=http://localhost:4586 logs create-log-group \
  --log-group-name /myapp/logs
```

---

## Lambda Hot Reload

Drop a `.zip` file into the `functions/` folder — CloudDev auto-registers it within 2 seconds:
```bash
zip functions/my-function.zip my-function.py
# Auto-loaded function: my-function ✅
```

---

## Data Persistence

S3 and DynamoDB data survives restarts automatically. State is saved to `~/.clouddev/state.json` when you press `Ctrl+C`.

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

## Service Ports

| Service | Port |
|---|---|
| S3 | 4566 |
| DynamoDB | 4569 |
| Lambda | 4574 |
| SQS | 4576 |
| API Gateway | 4572 |
| SNS | 4575 |
| Secrets Manager | 4584 |
| CloudWatch Logs | 4586 |
| Dashboard | 4580 |

---

## Why not LocalStack?

LocalStack recently ended its free Community Edition — requiring an account and moving to a paid model. CloudDev is built to be the free, open alternative forever.

| Feature | CloudDev | LocalStack Free |
|---|---|---|
| Requires account | ❌ No | ✅ Yes |
| Open source | ✅ Yes | ⚠️ Partial |
| S3, DynamoDB, Lambda | ✅ Yes | ✅ Yes |
| SNS, Secrets Manager | ✅ Yes | ❌ Paid only |
| Web dashboard | ✅ Yes | ❌ No |
| IaC auto-detection | ✅ Yes | ❌ No |
| Cost estimator | ✅ Yes | ❌ No |
| Data persistence | ✅ Yes | ❌ No |
| Cost | 🆓 Free forever | 💰 Paid |

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a PR.

---

## License

Apache 2.0 — free to use, forever.

---

*Built with Go · Claude AI (architect) · OpenAI Codex (developer)*
