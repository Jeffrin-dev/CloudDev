package costestimator

import (
	"testing"

	"github.com/clouddev/clouddev/internal/config"
)

func TestCalculateIncludesEnabledServices(t *testing.T) {
	cfg := &config.Config{Services: config.Services{
		S3:         true,
		DynamoDB:   true,
		Lambda:     true,
		SQS:        true,
		APIGateway: true,
	}}

	estimate := Calculate(cfg)

	if len(estimate.Services) != 5 {
		t.Fatalf("expected 5 enabled services, got %d", len(estimate.Services))
	}
	if estimate.Total != 28.00 {
		t.Fatalf("expected total 28.00, got %.2f", estimate.Total)
	}
}

func TestCalculateSumsEnabledServicesOnly(t *testing.T) {
	cfg := &config.Config{Services: config.Services{
		Lambda:         true,
		SNS:            true,
		CloudWatchLogs: true,
	}}

	estimate := Calculate(cfg)

	if len(estimate.Services) != 3 {
		t.Fatalf("expected 3 enabled services, got %d", len(estimate.Services))
	}
	if estimate.Total != 8.00 {
		t.Fatalf("expected total 8.00, got %.2f", estimate.Total)
	}
	if estimate.Services[0].Name != "Lambda" || estimate.Services[1].Name != "SNS" || estimate.Services[2].Name != "CloudWatch Logs" {
		t.Fatalf("unexpected service order: %#v", estimate.Services)
	}
}
