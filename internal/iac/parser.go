package iac

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParseResult struct {
	Services map[string]bool
}

func ParseTerraform(dir string) (*ParseResult, error) {
	result := newParseResult()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}

		file, err := os.Open(dir + string(os.PathSeparator) + entry.Name())
		if err != nil {
			return nil, err
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			resourceType := terraformResourceType(line)
			enableServiceForTerraformResource(result.Services, resourceType)
		}
		closeErr := file.Close()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}

	return result, nil
}

func ParseCloudFormation(file string) (*ParseResult, error) {
	result := newParseResult()

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	ext := strings.ToLower(file)
	if strings.HasSuffix(ext, ".json") {
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return nil, err
		}
	}

	resourcesRaw, ok := root["Resources"]
	if !ok {
		return result, nil
	}

	resources, ok := resourcesRaw.(map[string]any)
	if !ok {
		return result, nil
	}

	for _, resRaw := range resources {
		resMap, ok := resRaw.(map[string]any)
		if !ok {
			continue
		}
		typeValue, ok := resMap["Type"].(string)
		if !ok {
			continue
		}
		enableServiceForCloudFormationType(result.Services, typeValue)
	}

	return result, nil
}

func newParseResult() *ParseResult {
	return &ParseResult{Services: map[string]bool{
		"s3":          false,
		"dynamodb":    false,
		"lambda":      false,
		"sqs":         false,
		"api_gateway": false,
	}}
}

func terraformResourceType(line string) string {
	if !strings.HasPrefix(line, "resource ") {
		return ""
	}
	firstQuote := strings.Index(line, "\"")
	if firstQuote < 0 {
		return ""
	}
	rest := line[firstQuote+1:]
	secondQuote := strings.Index(rest, "\"")
	if secondQuote < 0 {
		return ""
	}
	return rest[:secondQuote]
}

func enableServiceForTerraformResource(services map[string]bool, resourceType string) {
	switch resourceType {
	case "aws_s3_bucket":
		services["s3"] = true
	case "aws_dynamodb_table":
		services["dynamodb"] = true
	case "aws_lambda_function":
		services["lambda"] = true
	case "aws_sqs_queue":
		services["sqs"] = true
	case "aws_api_gateway_rest_api":
		services["api_gateway"] = true
	}
}

func enableServiceForCloudFormationType(services map[string]bool, cfType string) {
	switch cfType {
	case "AWS::S3::Bucket":
		services["s3"] = true
	case "AWS::DynamoDB::Table":
		services["dynamodb"] = true
	case "AWS::Lambda::Function":
		services["lambda"] = true
	case "AWS::SQS::Queue":
		services["sqs"] = true
	case "AWS::ApiGateway::RestApi":
		services["api_gateway"] = true
	}
}
