package cmd

import (
	"fmt"
	"os"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/clouddev/clouddev/internal/costestimator"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var estimateCmd = &cobra.Command{
	Use:   "estimate",
	Short: "Estimate monthly AWS costs for enabled CloudDev services",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig("clouddev.yml")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		estimate := costestimator.Calculate(cfg)
		fmt.Println("CloudDev Cost Estimator")
		fmt.Println("=======================")
		fmt.Printf("%-15s %s\n", "Service", "Estimated Monthly Cost")
		for _, service := range estimate.Services {
			fmt.Printf("%-15s $%.2f\n", service.Name, service.Cost)
		}
		fmt.Println("-----------------------")
		fmt.Printf("%-15s $%.2f\n\n", "Total", estimate.Total)
		color.New(color.FgGreen).Printf("💰 Running locally with CloudDev saves you ~$%.2f/month!\n", estimate.Total)
	},
}

func init() {
	rootCmd.AddCommand(estimateCmd)
}
