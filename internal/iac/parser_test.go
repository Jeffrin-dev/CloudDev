package iac

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTerraformDetectsS3AndLambda(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tfContent := `resource "aws_s3_bucket" "assets" {}
resource "aws_lambda_function" "processor" {}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(tfContent), 0o644); err != nil {
		t.Fatalf("write tf file: %v", err)
	}

	result, err := ParseTerraform(dir)
	if err != nil {
		t.Fatalf("ParseTerraform: %v", err)
	}

	if !result.Services["s3"] {
		t.Fatalf("expected s3 to be detected")
	}
	if !result.Services["lambda"] {
		t.Fatalf("expected lambda to be detected")
	}
	if result.Services["dynamodb"] {
		t.Fatalf("did not expect dynamodb to be detected")
	}
}

func TestParseCloudFormationDetectsDynamoDBAndSQS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "template.yaml")

	template := `Resources:
  OrdersTable:
    Type: AWS::DynamoDB::Table
  JobsQueue:
    Type: AWS::SQS::Queue
`
	if err := os.WriteFile(templatePath, []byte(template), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	result, err := ParseCloudFormation(templatePath)
	if err != nil {
		t.Fatalf("ParseCloudFormation: %v", err)
	}

	if !result.Services["dynamodb"] {
		t.Fatalf("expected dynamodb to be detected")
	}
	if !result.Services["sqs"] {
		t.Fatalf("expected sqs to be detected")
	}
	if result.Services["s3"] {
		t.Fatalf("did not expect s3 to be detected")
	}
}

func TestParseKubernetesDetectsS3FromMinioImage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifest := `kind: Deployment
spec:
  template:
    spec:
      containers:
        - image: minio/minio:latest
`
	if err := os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := ParseKubernetes(dir)
	if err != nil {
		t.Fatalf("ParseKubernetes: %v", err)
	}
	if !result.Services["s3"] {
		t.Fatalf("expected s3 to be detected")
	}
}

func TestParseKubernetesDetectsLambdaFromLambdaImage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifest := `kind: Deployment
spec:
  template:
    spec:
      containers:
        - image: local/lambda-runtime:latest
`
	if err := os.WriteFile(filepath.Join(dir, "lambda.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := ParseKubernetes(dir)
	if err != nil {
		t.Fatalf("ParseKubernetes: %v", err)
	}
	if !result.Services["lambda"] {
		t.Fatalf("expected lambda to be detected")
	}
}

func TestParseKubernetesDetectsDynamoDBFromServicePort4569(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifest := `kind: Service
spec:
  ports:
    - port: 4569
`
	if err := os.WriteFile(filepath.Join(dir, "service.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := ParseKubernetes(dir)
	if err != nil {
		t.Fatalf("ParseKubernetes: %v", err)
	}
	if !result.Services["dynamodb"] {
		t.Fatalf("expected dynamodb to be detected")
	}
}
