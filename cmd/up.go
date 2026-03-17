package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/clouddev/clouddev/internal/dashboard"
	"github.com/clouddev/clouddev/internal/docker"
	"github.com/clouddev/clouddev/internal/services/apigateway"
	"github.com/clouddev/clouddev/internal/services/dynamodb"
	"github.com/clouddev/clouddev/internal/services/lambda"
	"github.com/clouddev/clouddev/internal/services/s3"
	"github.com/clouddev/clouddev/internal/services/sqs"
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
		if cfg.Services.SQS {
			go func() {
				if err := sqs.Start(cfg.Ports.SQS); err != nil {
					fmt.Fprintf(os.Stderr, "SQS server error: %v\n", err)
				}
			}()
			printSuccess("SQS server starting on port %d", cfg.Ports.SQS)
		}
		if cfg.Services.APIGateway {
			go func() {
				if err := apigateway.Start(cfg.Ports.APIGateway, cfg.Ports.Lambda); err != nil {
					fmt.Fprintf(os.Stderr, "API Gateway server error: %v\n", err)
				}
			}()
			printSuccess("API Gateway starting on port %d", cfg.Ports.APIGateway)
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
		go func() {
			serviceMap := map[string]int{
				"s3":          cfg.Ports.S3,
				"dynamodb":    cfg.Ports.DynamoDB,
				"lambda":      cfg.Ports.Lambda,
				"sqs":         cfg.Ports.SQS,
				"api_gateway": cfg.Ports.APIGateway,
			}
			if err := dashboard.Start(4580, serviceMap); err != nil {
				fmt.Fprintf(os.Stderr, "Dashboard error: %v\n", err)
			}
		}()
		printSuccess("Dashboard running at http://localhost:4580")
		printInfo("CloudDev is running. Press Ctrl+C to stop...")
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		printInfo("Shutting down...")
		return nil
	},
}

func buildServiceOptions(cfg *config.Config) []docker.ContainerOptions {
	services := make([]docker.ContainerOptions, 0)
	return services
}

func init() {
	rootCmd.AddCommand(upCmd)
}
