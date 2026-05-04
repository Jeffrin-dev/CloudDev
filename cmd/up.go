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
	"github.com/clouddev/clouddev/internal/persist"
	"github.com/clouddev/clouddev/internal/services/apigateway"
	"github.com/clouddev/clouddev/internal/services/apigatewayv2"
	"github.com/clouddev/clouddev/internal/services/appsync"
	"github.com/clouddev/clouddev/internal/services/athena"
	"github.com/clouddev/clouddev/internal/services/acm"
	"github.com/clouddev/clouddev/internal/services/bedrock"
	"github.com/clouddev/clouddev/internal/services/cloudformation"
	"github.com/clouddev/clouddev/internal/services/cloudfront"
	"github.com/clouddev/clouddev/internal/services/cloudwatchevents"
	"github.com/clouddev/clouddev/internal/services/cloudwatchlogs"
	"github.com/clouddev/clouddev/internal/services/cloudwatchmetrics"
	"github.com/clouddev/clouddev/internal/services/cognito"
	"github.com/clouddev/clouddev/internal/services/dynamodb"
	"github.com/clouddev/clouddev/internal/services/dynamodbstreams"
	"github.com/clouddev/clouddev/internal/services/ecr"
	"github.com/clouddev/clouddev/internal/services/ecs"
	"github.com/clouddev/clouddev/internal/services/elasticache"
	"github.com/clouddev/clouddev/internal/services/eventbridge"
	"github.com/clouddev/clouddev/internal/services/firehose"
	"github.com/clouddev/clouddev/internal/services/iam"
	"github.com/clouddev/clouddev/internal/services/kinesis"
	"github.com/clouddev/clouddev/internal/services/kms"
	"github.com/clouddev/clouddev/internal/services/lambda"
	"github.com/clouddev/clouddev/internal/services/lambdalayers"
	"github.com/clouddev/clouddev/internal/services/lambdaurls"
	"github.com/clouddev/clouddev/internal/services/rekognition"
	"github.com/clouddev/clouddev/internal/services/route53"
	"github.com/clouddev/clouddev/internal/services/s3"
	"github.com/clouddev/clouddev/internal/services/secretsmanager"
	"github.com/clouddev/clouddev/internal/services/ses"
	"github.com/clouddev/clouddev/internal/services/sqs"
	"github.com/clouddev/clouddev/internal/services/ssm"
	"github.com/clouddev/clouddev/internal/services/stepfunctions"
	"github.com/clouddev/clouddev/internal/services/sts"
	"github.com/clouddev/clouddev/internal/services/xray"
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
		state := persist.State{}
		if err := persist.Load(&state); err != nil {
			printWarning("State file is corrupted, starting fresh: %v", err)
			state = persist.State{}
		}
		s3.LoadState(state.S3)
		dynamodb.LoadState(state.DynamoDB)
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
			go func() {
				if err := dynamodbstreams.Start(4570); err != nil {
					fmt.Fprintf(os.Stderr, "DynamoDB Streams server error: %v\n", err)
				}
			}()
			printSuccess("DynamoDB Streams server starting on port %d", 4570)
		}
		if cfg.Services.Lambda {
			go func() {
				if err := lambda.Start(cfg.Ports.Lambda, cfg.Lambda.FunctionsDir, cfg.Lambda.HotReload); err != nil {
					fmt.Fprintf(os.Stderr, "Lambda server error: %v\n", err)
				}
			}()
			printSuccess("Lambda server starting on port %d", cfg.Ports.Lambda)
		}
		go func() {
			if err := lambdaurls.Start(4595, cfg.Ports.Lambda); err != nil {
				fmt.Fprintf(os.Stderr, "Lambda Function URLs server error: %v\n", err)
			}
		}()
		printSuccess("Lambda Function URLs server starting on port %d", 4595)
		go func() {
			if err := lambdalayers.Start(4578); err != nil {
				fmt.Fprintf(os.Stderr, "Lambda Layers server error: %v\n", err)
			}
		}()
		printSuccess("Lambda Layers server starting on port %d", 4578)
		if cfg.Services.SQS {
			go func() {
				if err := sqs.Start(cfg.Ports.SQS); err != nil {
					fmt.Fprintf(os.Stderr, "SQS server error: %v\n", err)
				}
			}()
			printSuccess("SQS server starting on port %d", cfg.Ports.SQS)
		}
		go func() {
			if err := ses.Start(4579); err != nil {
				fmt.Fprintf(os.Stderr, "SES server error: %v\n", err)
			}
		}()
		printSuccess("SES server starting on port %d", 4579)
		go func() {
			if err := acm.Start(4560); err != nil {
				fmt.Fprintf(os.Stderr, "ACM server error: %v\n", err)
			}
		}()
		printSuccess("ACM server starting on port %d", 4560)
		if cfg.Services.APIGateway {
			go func() {
				if err := apigateway.Start(cfg.Ports.APIGateway, cfg.Ports.Lambda); err != nil {
					fmt.Fprintf(os.Stderr, "API Gateway server error: %v\n", err)
				}
			}()
			printSuccess("API Gateway starting on port %d", cfg.Ports.APIGateway)
			go func() {
				if err := apigatewayv2.Start(4573); err != nil {
					fmt.Fprintf(os.Stderr, "API Gateway v2 server error: %v\n", err)
				}
			}()
			printSuccess("API Gateway v2 server starting on port %d", 4573)
		}
		go func() {
			if err := appsync.Start(4567); err != nil {
				fmt.Fprintf(os.Stderr, "AppSync server error: %v\n", err)
			}
		}()
		printSuccess("AppSync server starting on port %d", 4567)
		go func() {
			if err := secretsmanager.Start(4584); err != nil {
				fmt.Fprintf(os.Stderr, "Secrets Manager server error: %v\n", err)
			}
		}()
		printSuccess("Secrets Manager server starting on port %d", 4584)
		go func() {
			if err := ssm.Start(4583); err != nil {
				fmt.Fprintf(os.Stderr, "SSM server error: %v\n", err)
			}
		}()
		printSuccess("SSM server starting on port %d", 4583)
		go func() {
			if err := stepfunctions.Start(4585); err != nil {
				fmt.Fprintf(os.Stderr, "Step Functions server error: %v\n", err)
			}
		}()
		printSuccess("Step Functions server starting on port %d", 4585)
		go func() {
			if err := eventbridge.Start(4587, cfg.Ports.Lambda, cfg.Ports.SQS); err != nil {
				fmt.Fprintf(os.Stderr, "EventBridge server error: %v\n", err)
			}
		}()
		printSuccess("EventBridge server starting on port %d", 4587)
		go func() {
			if err := xray.Start(4588); err != nil {
				fmt.Fprintf(os.Stderr, "X-Ray server error: %v\n", err)
			}
		}()
		printSuccess("X-Ray server starting on port %d", 4588)
		go func() {
			if err := route53.Start(4589); err != nil {
				fmt.Fprintf(os.Stderr, "Route53 server error: %v\n", err)
			}
		}()
		printSuccess("Route53 server starting on port %d", 4589)
		if true {
			go func() {
				if err := cloudwatchlogs.Start(4586); err != nil {
					fmt.Fprintf(os.Stderr, "CloudWatch Logs error: %v\n", err)
				}
			}()
			printSuccess("CloudWatch Logs starting on port 4586")
		}
		if true {
			go func() {
				if err := cloudwatchmetrics.Start(4582); err != nil {
					fmt.Fprintf(os.Stderr, "CloudWatch Metrics error: %v\n", err)
				}
			}()
			printSuccess("CloudWatch Metrics starting on port 4582")
		}
		go func() {
			if err := cloudwatchevents.Start(4590); err != nil {
				fmt.Fprintf(os.Stderr, "CloudWatch Events error: %v\n", err)
			}
		}()
		printSuccess("CloudWatch Events server starting on port %d", 4590)
		go func() {
			if err := iam.Start(4593); err != nil {
				fmt.Fprintf(os.Stderr, "IAM server error: %v\n", err)
			}
		}()
		printSuccess("IAM server starting on port %d", 4593)
		go func() {
			if err := sts.Start(4592); err != nil {
				fmt.Fprintf(os.Stderr, "STS server error: %v\n", err)
			}
		}()
		printSuccess("STS server starting on port %d", 4592)
		go func() {
			if err := kms.Start(4599); err != nil {
				fmt.Fprintf(os.Stderr, "KMS server error: %v\n", err)
			}
		}()
		printSuccess("KMS server starting on port %d", 4599)
		go func() {
			if err := athena.Start(4564); err != nil {
				fmt.Fprintf(os.Stderr, "Athena server error: %v\n", err)
			}
		}()
		printSuccess("Athena server starting on port %d", 4564)
		go func() {
			if err := cloudformation.Start(4581); err != nil {
				fmt.Fprintf(os.Stderr, "CloudFormation server error: %v\n", err)
			}
		}()
		printSuccess("CloudFormation server starting on port %d", 4581)
		go func() {
			if err := cloudfront.Start(4563); err != nil {
				fmt.Fprintf(os.Stderr, "CloudFront server error: %v\n", err)
			}
		}()
		printSuccess("CloudFront server starting on port %d", 4563)
		go func() {
			if err := elasticache.Start(4598, 4597); err != nil {
				fmt.Fprintf(os.Stderr, "ElastiCache server error: %v\n", err)
			}
		}()
		printSuccess("ElastiCache server starting on ports %d (redis) and %d (http)", 4598, 4597)
		go func() {
			if err := cognito.Start(4596); err != nil {
				fmt.Fprintf(os.Stderr, "Cognito server error: %v\n", err)
			}
		}()
		printSuccess("Cognito server starting on port %d", 4596)
		go func() {
			if err := rekognition.Start(4594); err != nil {
				fmt.Fprintf(os.Stderr, "Rekognition server error: %v\n", err)
			}
		}()
		printSuccess("Rekognition server starting on port %d", 4594)
		go func() {
			if err := bedrock.Start(4591); err != nil {
				fmt.Fprintf(os.Stderr, "Bedrock server error: %v\n", err)
			}
		}()
		printSuccess("Bedrock server starting on port %d", 4591)
		go func() {
			if err := kinesis.Start(4568); err != nil {
				fmt.Fprintf(os.Stderr, "Kinesis server error: %v\n", err)
			}
		}()
		printSuccess("Kinesis server starting on port %d", 4568)
		go func() {
			if err := firehose.Start(4571); err != nil {
				fmt.Fprintf(os.Stderr, "Firehose server error: %v\n", err)
			}
		}()
		printSuccess("Firehose server starting on port %d", 4571)
		go func() {
			if err := ecs.Start(4561); err != nil {
				fmt.Fprintf(os.Stderr, "ECS server error: %v\n", err)
			}
		}()
		printSuccess("ECS server starting on port %d", 4561)
		go func() {
			if err := ecr.Start(4562); err != nil {
				fmt.Fprintf(os.Stderr, "ECR server error: %v\n", err)
			}
		}()
		printSuccess("ECR server starting on port %d", 4562)
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
				"s3":              cfg.Ports.S3,
				"dynamodb":        cfg.Ports.DynamoDB,
				"lambda":          cfg.Ports.Lambda,
				"lambda_layers":   4578,
				"sqs":             cfg.Ports.SQS,
				"ses":             4579,
				"api_gateway":     cfg.Ports.APIGateway,
				"sns":             4575,
				"secrets_manager": 4584,
				"ssm":             4583,
				"cloudwatch_logs": 4586,
				// v0.4.0 additions:
				"cloudwatch_metrics":   4582,
				"cloudwatch_events":    4590,
				"xray":                 4588,
				"route53":              4589,
				"iam":                  4593,
				"sts":                  4592,
				"bedrock":              4591,
				"firehose":             4571,
				"kms":                  4599,
				"cloudformation":       4581,
				"step_functions":       4585,
				"eventbridge":          4587,
				"elasticache":          4598,
				"elasticache_http":     4597,
				"cognito":              4596,
				"rekognition":          4594,
				"lambda_function_urls": 4595,
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
		printInfo("Saving state...")
		stateToSave := map[string]interface{}{
			"s3":       s3.GetState(),
			"dynamodb": dynamodb.GetState(),
		}
		if err := persist.Save(stateToSave); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save state: %v\n", err)
		} else {
			printSuccess("State saved to ~/.clouddev/state.json")
		}
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
