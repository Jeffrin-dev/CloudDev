package cmd

import (
	"fmt"
	"net"
	"os"
	"text/tabwriter"
	"time"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type serviceStatus struct {
	name    string
	port    int
	running bool
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check whether enabled local cloud services are running",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig("clouddev.yml")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		statuses := enabledServiceStatuses(cfg)
		allRunning := true

		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "Service\tPort\tStatus")
		for _, service := range statuses {
			statusText := color.New(color.FgRed).Sprint("Stopped")
			if service.running {
				statusText = color.New(color.FgGreen).Sprint("Running")
			} else {
				allRunning = false
			}

			fmt.Fprintf(writer, "%s\t%d\t%s\n", service.name, service.port, statusText)
		}
		_ = writer.Flush()

		if allRunning {
			os.Exit(0)
		}
		os.Exit(1)
	},
}

func enabledServiceStatuses(cfg *config.Config) []serviceStatus {
	statuses := make([]serviceStatus, 0, 5)

	if cfg.Services.S3 {
		statuses = append(statuses, newServiceStatus("S3", cfg.Ports.S3))
	}
	if cfg.Services.DynamoDB {
		statuses = append(statuses, newServiceStatus("DynamoDB", cfg.Ports.DynamoDB))
	}
	if cfg.Services.Lambda {
		statuses = append(statuses, newServiceStatus("Lambda", cfg.Ports.Lambda))
	}
	if cfg.Services.SQS {
		statuses = append(statuses, newServiceStatus("SQS", cfg.Ports.SQS))
	}
	if cfg.Services.APIGateway {
		statuses = append(statuses, newServiceStatus("API Gateway", cfg.Ports.APIGateway))
	}

	return statuses
}

func newServiceStatus(name string, port int) serviceStatus {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", address, time.Second)
	running := err == nil
	if conn != nil {
		_ = conn.Close()
	}

	return serviceStatus{
		name:    name,
		port:    port,
		running: running,
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
