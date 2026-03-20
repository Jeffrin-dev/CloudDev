package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	DefaultS3Port         = 4566
	DefaultLambdaPort     = 4574
	DefaultDynamoDBPort   = 4569
	DefaultSQSPort        = 4576
	DefaultAPIGatewayPort = 4572
)

type Services struct {
	S3             bool `yaml:"s3"`
	DynamoDB       bool `yaml:"dynamodb"`
	Lambda         bool `yaml:"lambda"`
	SQS            bool `yaml:"sqs"`
	APIGateway     bool `yaml:"api_gateway"`
	SNS            bool `yaml:"sns"`
	SecretsManager bool `yaml:"secrets_manager"`
	CloudWatchLogs bool `yaml:"cloudwatch_logs"`
}

type Ports struct {
	S3         int `yaml:"s3"`
	Lambda     int `yaml:"lambda"`
	DynamoDB   int `yaml:"dynamodb"`
	SQS        int `yaml:"sqs"`
	APIGateway int `yaml:"api_gateway"`
}

type Lambda struct {
	HotReload    bool   `yaml:"hot_reload"`
	FunctionsDir string `yaml:"functions_dir"`
}

type Config struct {
	Services Services `yaml:"services"`
	Ports    Ports    `yaml:"ports"`
	Lambda   Lambda   `yaml:"lambda"`
}

func (c Services) AnyEnabled() bool {
	return c.S3 || c.DynamoDB || c.Lambda || c.SQS || c.APIGateway || c.SNS || c.SecretsManager || c.CloudWatchLogs
}

func (c *Config) applyDefaultPorts() {
	if c.Ports.S3 == 0 {
		c.Ports.S3 = DefaultS3Port
	}
	if c.Ports.Lambda == 0 {
		c.Ports.Lambda = DefaultLambdaPort
	}
	if c.Ports.DynamoDB == 0 {
		c.Ports.DynamoDB = DefaultDynamoDBPort
	}
	if c.Ports.SQS == 0 {
		c.Ports.SQS = DefaultSQSPort
	}
	if c.Ports.APIGateway == 0 {
		c.Ports.APIGateway = DefaultAPIGatewayPort
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("clouddev config file not found at %q; run 'clouddev init <project-name>' to create one", path)
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	cfg.applyDefaultPorts()

	if !cfg.Services.AnyEnabled() {
		return nil, fmt.Errorf("invalid clouddev config %q: at least one service must be enabled", path)
	}

	return cfg, nil
}
