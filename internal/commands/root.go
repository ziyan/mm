package commands

import (
	"github.com/spf13/cobra"
	mmlog "github.com/ziyan/mm/internal/logging"
	"github.com/ziyan/mm/internal/printer"
)

var rootCommand = &cobra.Command{
	Use:   "mm",
	Short: "Mattermost CLI client",
	Long:  "A command-line client for Mattermost, similar to gh for GitHub.",
	PersistentPreRun: func(command *cobra.Command, args []string) {
		jsonFlag, _ := command.Flags().GetBool("json")
		printer.JSONOutput = jsonFlag

		logLevel, _ := command.Flags().GetString("log-level")
		mmlog.Setup(logLevel)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCommand.PersistentFlags().Bool("json", false, "Output in JSON format")
	rootCommand.PersistentFlags().String("token", "", "Override access token")
	rootCommand.PersistentFlags().String("server", "", "Override server URL")
	rootCommand.PersistentFlags().StringP("team", "T", "", "Override active team (by name)")
	rootCommand.PersistentFlags().StringP("log-level", "l", "WARNING", "Log level (DEBUG, INFO, WARNING, ERROR, CRITICAL)")
}

func Execute() error {
	return rootCommand.Execute()
}
