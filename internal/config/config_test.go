package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "clouddev.yml")

	content := `services:
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
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if !cfg.Services.S3 || !cfg.Services.DynamoDB || !cfg.Services.Lambda || cfg.Services.SQS || !cfg.Services.APIGateway {
		t.Fatalf("services were not loaded correctly: %+v", cfg.Services)
	}

	if cfg.Ports.S3 != 4566 || cfg.Ports.Lambda != 4574 || cfg.Ports.DynamoDB != 4569 || cfg.Ports.SQS != 4576 || cfg.Ports.APIGateway != 4572 {
		t.Fatalf("ports were not loaded correctly: %+v", cfg.Ports)
	}

	if !cfg.Lambda.HotReload || cfg.Lambda.FunctionsDir != "./functions" {
		t.Fatalf("lambda config was not loaded correctly: %+v", cfg.Lambda)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	t.Parallel()
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected helpful not found message, got: %v", err)
	}
}

func TestLoadConfigAppliesDefaultPortsWhenOmitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "clouddev.yml")

	content := `services:
  s3: true
  dynamodb: false
  lambda: false
  sqs: false
  api_gateway: false

lambda:
  hot_reload: true
  functions_dir: ./functions
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ports.S3 != DefaultS3Port ||
		cfg.Ports.Lambda != DefaultLambdaPort ||
		cfg.Ports.DynamoDB != DefaultDynamoDBPort ||
		cfg.Ports.SQS != DefaultSQSPort ||
		cfg.Ports.APIGateway != DefaultAPIGatewayPort {
		t.Fatalf("expected default ports, got: %+v", cfg.Ports)
	}
}

func TestLoadConfigErrorsWhenNoServicesEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "clouddev.yml")

	content := `services:
  s3: false
  dynamodb: false
  lambda: false
  sqs: false
  api_gateway: false
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error when no services are enabled")
	}
	if !strings.Contains(err.Error(), "at least one service") {
		t.Fatalf("expected service validation error, got: %v", err)
	}
}
