package cmd

import (
	"context"
	"os"

	"github.com/clouddev/clouddev/internal/docker"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all running local cloud services",
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := docker.NewManager(os.Stdout)
		if err != nil {
			return err
		}

		if err := manager.StopAll(context.Background()); err != nil {
			return err
		}

		printSuccess("Stopped all managed clouddev containers")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
