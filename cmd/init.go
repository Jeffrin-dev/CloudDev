package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const clouddevTemplate = `services:
  s3: true
  dynamodb: true
  lambda: true
  sqs: false
  api_gateway: true

ports:
  s3: 4566
  lambda: 4574
  dynamodb: 4569
  sqs: 4576
  api_gateway: 4572

lambda:
  hot_reload: true
  functions_dir: ./functions
`

var initCmd = &cobra.Command{
	Use:   "init <project-name>",
	Short: "Initialize a new CloudDev project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		projectPath := filepath.Join(".", projectName)

		printVerbose("Creating project at %s", projectPath)

		if _, err := os.Stat(projectPath); err == nil {
			return printError("project '%s' already exists", projectName)
		} else if !os.IsNotExist(err) {
			return printError("could not inspect project path: %v", err)
		}

		dirs := []string{
			projectPath,
			filepath.Join(projectPath, "functions"),
			filepath.Join(projectPath, "infrastructure"),
		}

		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return printError("failed to create directory '%s': %v", dir, err)
			}
			printVerbose("Created directory: %s", dir)
		}

		configPath := filepath.Join(projectPath, "clouddev.yml")
		content := []byte(clouddevTemplate)
		if err := os.WriteFile(configPath, content, 0o644); err != nil {
			return printError("failed to write '%s': %v", configPath, err)
		}

		printSuccess("Initialized CloudDev project: %s", projectName)
		printInfo("Created: %s", configPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
