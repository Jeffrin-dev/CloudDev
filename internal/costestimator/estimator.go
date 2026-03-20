package costestimator

import (
	"sort"

	"github.com/clouddev/clouddev/internal/config"
)

type ServiceEstimate struct {
	Name  string
	Cost  float64
	Order int
}

type Estimate struct {
	Services []ServiceEstimate
	Total    float64
}

var monthlyBaseCosts = []ServiceEstimate{
	{Name: "S3", Cost: 5.00, Order: 0},
	{Name: "DynamoDB", Cost: 10.00, Order: 1},
	{Name: "Lambda", Cost: 3.00, Order: 2},
	{Name: "SQS", Cost: 2.00, Order: 3},
	{Name: "API Gateway", Cost: 8.00, Order: 4},
	{Name: "SNS", Cost: 2.00, Order: 5},
	{Name: "Secrets Manager", Cost: 4.00, Order: 6},
	{Name: "CloudWatch Logs", Cost: 3.00, Order: 7},
}

func Calculate(cfg *config.Config) Estimate {
	enabled := enabledServices(cfg)
	services := make([]ServiceEstimate, 0, len(enabled))
	total := 0.0

	for _, service := range monthlyBaseCosts {
		if enabled[service.Name] {
			services = append(services, service)
			total += service.Cost
		}
	}

	sort.SliceStable(services, func(i, j int) bool {
		return services[i].Order < services[j].Order
	})

	return Estimate{Services: services, Total: total}
}

func enabledServices(cfg *config.Config) map[string]bool {
	return map[string]bool{
		"S3":              cfg.Services.S3,
		"DynamoDB":        cfg.Services.DynamoDB,
		"Lambda":          cfg.Services.Lambda,
		"SQS":             cfg.Services.SQS,
		"API Gateway":     cfg.Services.APIGateway,
		"SNS":             cfg.Services.SNS,
		"Secrets Manager": cfg.Services.SecretsManager,
		"CloudWatch Logs": cfg.Services.CloudWatchLogs,
	}
}
