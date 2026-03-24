# Contributing to CloudDev

Thank you for your interest in contributing to CloudDev! This document will help you get started.

---

## 🧭 Ways to Contribute

- **Bug fixes** — fix issues in existing service emulators
- **New services** — add missing AWS services
- **Improve parity** — make existing emulators more AWS-compatible
- **Tests** — add missing test coverage
- **Documentation** — improve README, comments, or examples
- **Dashboard** — improve the web UI

---

## 🛠️ Development Setup

### Prerequisites

- Go 1.22+
- Git
- AWS CLI (for live testing)
- `redis-cli` (for ElastiCache testing)

### Clone and Build

```bash
git clone https://github.com/Jeffrin-dev/CloudDev.git
cd CloudDev
go build -o clouddev .
go test ./...
```

---

## 📁 Project Structure

Each AWS service lives in its own package under `internal/services/`:

```
internal/services/
└── myservice/
    ├── server.go       # Service implementation
    └── server_test.go  # Tests
```

Every service must expose:

```go
func Start(port int) error
```

And be wired into `cmd/up.go`.

---

## 🧱 Adding a New Service

1. Create `internal/services/myservice/server.go`
2. Implement `Start(port int) error`
3. Add HTTP handler with correct protocol:
   - **JSON + X-Amz-Target** (KMS, Step Functions, EventBridge, Cognito, ElastiCache HTTP)
   - **Form-encoded + Action** (IAM, STS, CloudFormation, SQS, SNS, ElastiCache)
   - **REST** (S3, DynamoDB, Lambda)
4. Wire into `cmd/up.go`
5. Add tests in `server_test.go`
6. Update the dashboard service list in `internal/dashboard/server.go`

### Protocol Reference

| Service Type | Content-Type | Dispatch |
|---|---|---|
| JSON services | `application/x-amz-json-1.1` | `X-Amz-Target` header |
| Query services | `application/x-www-form-urlencoded` | `Action` form field |
| REST services | varies | HTTP method + path |

---

## ✅ Testing Guidelines

Every service must have tests covering:
- Happy path (basic create/list/describe)
- Error cases (not found, invalid input)
- Roundtrip operations (e.g. encrypt/decrypt, put/get)

Run tests:

```bash
go test ./...
```

Run tests for a specific service:

```bash
go test ./internal/services/kms/...
```

---

## 🔍 Live Testing

Always live test before submitting a PR:

```bash
./clouddev init test-app
cd test-app
../clouddev up &
sleep 3

# Run your AWS CLI tests here

kill %1 2>/dev/null
fuser -k 4566/tcp 4569/tcp ... 2>/dev/null
cd ..
rm -rf test-app
rm -f ~/.clouddev/state.json
```

---

## 📝 Pull Request Guidelines

- **One service per PR** — keep PRs focused
- **All tests must pass** — `go build ./...` and `go test ./...`
- **No new variables on left side of :=** — common Go mistake, double check
- **Match existing code style** — run `gofmt -w .` before committing
- **Update dashboard** — if adding a new service, add it to the dashboard service list
- **Descriptive commit messages** — e.g. `feat(kms): add KMS encrypt/decrypt`

---

## 🐛 Reporting Bugs

Please open a GitHub issue with:
- CloudDev version (`./clouddev --version`)
- Go version (`go version`)
- OS and architecture
- AWS CLI version (`aws --version`)
- Steps to reproduce
- Expected vs actual behavior

---

## 💡 Feature Requests

Open a GitHub issue with:
- The AWS service or feature you need
- Your use case
- Any relevant AWS documentation links

---

## 🏷️ Commit Message Format

```
type(scope): short description

Types: feat, fix, test, docs, refactor, chore
Scope: service name (s3, kms, cognito, dashboard, etc.)

Examples:
feat(kms): add GenerateDataKey operation
fix(stepfunctions): correct X-Amz-Target prefix
docs(readme): add ElastiCache usage example
```

---

## 📄 License

By contributing to CloudDev, you agree that your contributions will be licensed under the Apache 2.0 License.

---

Thank you for helping make CloudDev better! 🙏
