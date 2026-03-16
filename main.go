package main

import (
	"fmt"
	"os"

	"github.com/clouddev/clouddev/cmd"
	"github.com/fatih/color"
)

func main() {
	if err := cmd.Execute(); err != nil {
		color.New(color.FgRed).Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'clouddev --help' for usage.")
		os.Exit(1)
	}
}
