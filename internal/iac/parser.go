package iac

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func ParseKubernetes(dir string) (*ParseResult, error) {
	result := newParseResult()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		decoder := yaml.NewDecoder(bytes.NewReader(data))
		for {
			var resource map[string]any
			if err := decoder.Decode(&resource); err != nil {
				if err.Error() == "EOF" {
					break
				}
				return nil, err
			}
			if len(resource) == 0 {
				continue
			}
			parseKubernetesResource(result.Services, resource)
		}
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

func isYAMLFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func parseKubernetesResource(services map[string]bool, resource map[string]any) {
	kind, _ := resource["kind"].(string)
	switch kind {
	case "Deployment":
		for _, image := range deploymentImages(resource) {
			lower := strings.ToLower(image)
			switch {
			case strings.Contains(lower, "lambda"):
				services["lambda"] = true
			case strings.Contains(lower, "dynamodb"):
				services["dynamodb"] = true
			case strings.Contains(lower, "minio"), strings.Contains(lower, "s3"):
				services["s3"] = true
			}
		}
	case "Service":
		for _, port := range servicePorts(resource) {
			switch port {
			case 4566:
				services["s3"] = true
			case 4569:
				services["dynamodb"] = true
			case 4574:
				services["lambda"] = true
			case 4576:
				services["sqs"] = true
			}
		}
	case "ConfigMap":
		if configMapHasAWSKeys(resource) {
			for name, enabled := range services {
				if enabled {
					services[name] = true
				}
			}
		}
	case "Job":
		for _, image := range jobImages(resource) {
			if strings.Contains(strings.ToLower(image), "aws") {
				services["s3"] = true
				services["dynamodb"] = true
			}
		}
	}
}

func deploymentImages(resource map[string]any) []string {
	return podTemplateImages(resource, []string{"spec", "template", "spec", "containers"})
}

func jobImages(resource map[string]any) []string {
	return podTemplateImages(resource, []string{"spec", "template", "spec", "containers"})
}

func podTemplateImages(resource map[string]any, path []string) []string {
	current := any(resource)
	for _, key := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = mapping[key]
		if !ok {
			return nil
		}
	}
	containers, ok := current.([]any)
	if !ok {
		return nil
	}
	images := make([]string, 0, len(containers))
	for _, container := range containers {
		containerMap, ok := container.(map[string]any)
		if !ok {
			continue
		}
		image, _ := containerMap["image"].(string)
		if image != "" {
			images = append(images, image)
		}
	}
	return images
}

func servicePorts(resource map[string]any) []int {
	spec, ok := resource["spec"].(map[string]any)
	if !ok {
		return nil
	}
	ports, ok := spec["ports"].([]any)
	if !ok {
		return nil
	}
	values := make([]int, 0, len(ports))
	for _, port := range ports {
		portMap, ok := port.(map[string]any)
		if !ok {
			continue
		}
		number, ok := numericValue(portMap["port"])
		if ok {
			values = append(values, number)
		}
	}
	return values
}

func configMapHasAWSKeys(resource map[string]any) bool {
	data, ok := resource["data"].(map[string]any)
	if !ok {
		return false
	}
	for key := range data {
		if strings.HasPrefix(key, "AWS_") {
			return true
		}
	}
	return false
}

func numericValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
