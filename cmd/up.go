package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/clouddev/clouddev/internal/docker"
	"github.com/clouddev/clouddev/internal/services/dynamodb"
	"github.com/clouddev/clouddev/internal/services/s3"
	"github.com/clouddev/clouddev/internal/services/lambda"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start local cloud services",
	RunE: func(cmd *cobra.Command, args []string) error {
		printVerbose("Starting service orchestration in stub mode")
		printInfo("Bringing up local cloud services...")
		printWarning("Stub mode: Docker/service startup not implemented yet")
		printSuccess("Status: local services startup stub completed")
		cfg, err := config.LoadConfig("clouddev.yml")
		if err != nil {
			return err
		}
		if cfg.Services.S3 {
			go func() {
				if err := s3.Start(cfg.Ports.S3); err != nil {
					fmt.Fprintf(os.Stderr, "S3 server error: %v\n", err)
				}
			}()
			printSuccess("S3 server starting on port %d", cfg.Ports.S3)
		}
		if cfg.Services.DynamoDB {
			go func() {
				if err := dynamodb.Start(cfg.Ports.DynamoDB); err != nil {
					fmt.Fprintf(os.Stderr, "DynamoDB server error: %v\n", err)
				}
			}()
			printSuccess("DynamoDB server starting on port %d", cfg.Ports.DynamoDB)
		}
		if cfg.Services.Lambda {
           go func() {
               if err := lambda.Start(cfg.Ports.Lambda, cfg.Lambda.FunctionsDir, cfg.Lambda.HotReload); err != nil {
                   fmt.Fprintf(os.Stderr, "Lambda server error: %v\n", err)
               }
           }()
           printSuccess("Lambda server starting on port %d", cfg.Ports.Lambda)
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
		printInfo("CloudDev is running. Press Ctrl+C to stop...")
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		printInfo("Shutting down...")
		return nil
	},
}

func buildServiceOptions(cfg *config.Config) []docker.ContainerOptions {
	services := make([]docker.ContainerOptions, 0, 4)
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
