package cmd

import (
	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use: "vyalik",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
