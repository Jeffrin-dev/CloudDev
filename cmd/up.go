package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/clouddev/clouddev/internal/docker"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start local cloud services",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig("clouddev.yml")
		if err != nil {
			return err
		}

		manager, err := docker.NewManager(os.Stdout)
		if err != nil {
			return err
		}

		ctx := context.Background()
		services := buildServiceOptions(cfg)
		for _, service := range services {
			running, err := manager.IsRunning(ctx, service.Name)
			if err != nil {
				return err
			}
			if running {
				printWarning("Service %s is already running", service.Name)
				continue
			}

			id, err := manager.StartContainer(ctx, service)
			if err != nil {
				return err
			}
			printSuccess("Started %s (%s)", service.Name, id)
		}

		if cfg.Services.APIGateway {
			printInfo("api_gateway is enabled but managed in Go (no container started)")
		}

		return nil
	},
}

func buildServiceOptions(cfg *config.Config) []docker.ContainerOptions {
	services := make([]docker.ContainerOptions, 0, 4)

	if cfg.Services.S3 {
		services = append(services, docker.ContainerOptions{
			Name:        "clouddev-s3",
			Image:       "minio/minio",
			PortMapping: map[int]int{cfg.Ports.S3: cfg.Ports.S3},
			Labels:      map[string]string{"service": "s3"},
		})
	}
	if cfg.Services.DynamoDB {
		services = append(services, docker.ContainerOptions{
			Name:        "clouddev-dynamodb",
			Image:       "amazon/dynamodb-local",
			PortMapping: map[int]int{cfg.Ports.DynamoDB: cfg.Ports.DynamoDB},
			Labels:      map[string]string{"service": "dynamodb"},
		})
	}
	if cfg.Services.Lambda {
		services = append(services, docker.ContainerOptions{
			Name:  "clouddev-lambda",
			Image: "mlupin/docker-lambda",
			PortMapping: map[int]int{
				cfg.Ports.Lambda: cfg.Ports.Lambda,
			},
			EnvVars: map[string]string{
				"CLOUDDEV_HOT_RELOAD":    fmt.Sprintf("%t", cfg.Lambda.HotReload),
				"CLOUDDEV_FUNCTIONS_DIR": cfg.Lambda.FunctionsDir,
			},
			Labels: map[string]string{"service": "lambda"},
		})
	}
	if cfg.Services.SQS {
		services = append(services, docker.ContainerOptions{
			Name:        "clouddev-sqs",
			Image:       "softwaremill/elasticmq",
			PortMapping: map[int]int{cfg.Ports.SQS: cfg.Ports.SQS},
			Labels:      map[string]string{"service": "sqs"},
		})
	}

	return services
}

func init() {
	rootCmd.AddCommand(upCmd)
}
