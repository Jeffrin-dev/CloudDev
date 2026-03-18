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

## Reporting Issues

Please open a GitHub issue with:
- A clear description of the problem
- Steps to reproduce
- Expected vs actual behaviour
- Your OS and CloudDev version

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
