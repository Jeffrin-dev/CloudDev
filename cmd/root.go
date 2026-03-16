package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

var verbose bool

var rootCmd = &cobra.Command{
	Use:     "clouddev",
	Short:   "CloudDev helps you run AWS-like services locally",
	Long:    "CloudDev is a developer tool for running cloud service workflows locally.",
	Version: version,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.SetVersionTemplate(color.New(color.FgCyan).Sprintf("CloudDev version %s\n", version))
	cobra.EnableCommandSorting = true
}

func printVerbose(msg string, args ...interface{}) {
	if !verbose {
		return
	}
	color.New(color.FgHiBlack).Printf("[verbose] "+msg+"\n", args...)
}

func printInfo(msg string, args ...interface{}) {
	color.New(color.FgCyan).Printf(msg+"\n", args...)
}

func printSuccess(msg string, args ...interface{}) {
	color.New(color.FgGreen).Printf(msg+"\n", args...)
}

func printWarning(msg string, args ...interface{}) {
	color.New(color.FgYellow).Printf(msg+"\n", args...)
}

func printError(msg string, args ...interface{}) error {
	return fmt.Errorf(msg, args...)
}
