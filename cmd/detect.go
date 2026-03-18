package cmd

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/clouddev/clouddev/internal/iac"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect services from IaC templates and update clouddev.yml",
	RunE: func(cmd *cobra.Command, args []string) error {
		combined := map[string]bool{
			"s3":          false,
			"dynamodb":    false,
			"lambda":      false,
			"sqs":         false,
			"api_gateway": false,
		}

		tfResult, err := iac.ParseTerraform(".")
		if err != nil {
			return err
		}
		mergeServices(combined, tfResult.Services)

		for _, cfFile := range []string{"template.json", "template.yaml"} {
			if _, err := os.Stat(cfFile); err != nil {
				continue
			}
			cfResult, err := iac.ParseCloudFormation(cfFile)
			if err != nil {
				return err
			}
			mergeServices(combined, cfResult.Services)
		}

		enabled, err := updateCloudDevYAML("clouddev.yml", combined)
		if err != nil {
			return err
		}

		detected := enabledServiceNames(combined)
		if len(detected) == 0 {
			printInfo("No supported services detected from IaC files.")
			return nil
		}

		printInfo("Detected services: %v", detected)
		printSuccess("Enabled in clouddev.yml: %v", enabled)
		printInfo("Updated: %s", filepath.Clean("clouddev.yml"))
		return nil
	},
}

func mergeServices(dst map[string]bool, src map[string]bool) {
	for name, v := range src {
		if v {
			dst[name] = true
		}
	}
}

func updateCloudDevYAML(path string, detected map[string]bool) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := map[string]any{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	services, ok := cfg["services"].(map[string]any)
	if !ok {
		services = map[string]any{}
	}

	enabled := make([]string, 0)
	for name, isDetected := range detected {
		if !isDetected {
			continue
		}
		services[name] = true
		enabled = append(enabled, name)
	}
	sort.Strings(enabled)
	cfg["services"] = services

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return nil, err
	}
	return enabled, nil
}

func enabledServiceNames(services map[string]bool) []string {
	names := make([]string, 0)
	for name, enabled := range services {
		if enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func init() {
	rootCmd.AddCommand(detectCmd)
}
