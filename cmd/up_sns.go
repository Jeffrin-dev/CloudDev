package cmd

import (
	"fmt"
	"os"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/clouddev/clouddev/internal/services/sns"
	"github.com/spf13/cobra"
)

const snsPort = 4575

func init() {
	originalRunE := upCmd.RunE
	upCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig("clouddev.yml")
		if err != nil {
			return err
		}

		go func() {
			if err := sns.Start(snsPort, cfg.Ports.SQS); err != nil {
				fmt.Fprintf(os.Stderr, "SNS server error: %v\n", err)
			}
		}()
		printSuccess("SNS server starting on port %d", snsPort)

		return originalRunE(cmd, args)
	}
}
