# Contributing to CloudDev

Thank you for your interest in contributing! CloudDev is an 
open-source project and all contributions are welcome.

## Getting Started

1. Fork the repository
2. Clone your fork:
   `git clone https://github.com/YOUR_USERNAME/CloudDev`
3. Create a feature branch:
   `git checkout -b feature/my-feature`
4. Make your changes
5. Build and test:
   `go build -o clouddev . && go test ./...`
6. Commit:
   `git commit -m "feat: describe your change"`
7. Push:
   `git push origin feature/my-feature`
8. Open a Pull Request

## Commit Message Convention

We use Conventional Commits:

- `feat:` — new feature
- `fix:` — bug fix
- `docs:` — documentation only
- `chore:` — build/tooling changes
- `test:` — adding or updating tests

## Project Structure
```
clouddev/
  cmd/              # CLI commands (init, up, down, status, detect, estimate)
  internal/
    config/         # clouddev.yml parser
    costestimator/  # AWS cost estimation
    dashboard/      # Web dashboard server
    docker/         # Docker container manager
    iac/            # IaC parser (Terraform, CloudFormation, Kubernetes)
    persist/        # Data persistence
    services/
      apigateway/   # API Gateway emulation
      cloudwatchlogs/ # CloudWatch Logs emulation
      dynamodb/     # DynamoDB emulation
      lambda/       # Lambda emulation with hot reload
      s3/           # S3 emulation
      secretsmanager/ # Secrets Manager emulation
      sns/          # SNS emulation
      sqs/          # SQS emulation
  functions/        # User Lambda functions
  infrastructure/   # IaC definitions
```

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

## Development Setup

Requirements:
- Go 1.22+
- Docker
- AWS CLI (for testing)
```bash
git clone https://github.com/Jeffrin-dev/CloudDev
cd CloudDev
go mod tidy
go build -o clouddev .
./clouddev --version
```

## Running Tests
```bash
go test ./...
```

## Adding a New Service

1. Create `internal/services/<servicename>/server.go`
2. Implement `Start(port int) error`
3. Wire into `cmd/up.go`
4. Write tests in `internal/services/<servicename>/server_test.go`
5. Update `README.md` with the new service and port

## Reporting Issues

Please open a GitHub issue with:
- A clear description of the problem
- Steps to reproduce
- Expected vs actual behaviour
- Your OS and CloudDev version
