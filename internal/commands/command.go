package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	slashCommand := &cobra.Command{
		Use:     "slash",
		Aliases: []string{"command"},
		Short:   "Slash command operations",
	}

	executeCommand := &cobra.Command{
		Use:   "exec <channel> <command>",
		Short: "Execute a slash command",
		Long:  "Execute a slash command in a channel. Include the leading /.",
		Args:  cobra.MinimumNArgs(2),
		RunE:  slashExecuteRun,
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List custom slash commands",
		RunE:  slashListRun,
	}

	slashCommand.AddCommand(executeCommand, listCommand)
	rootCommand.AddCommand(slashCommand)
}

func slashExecuteRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	commandText := strings.Join(args[1:], " ")
	if !strings.HasPrefix(commandText, "/") {
		commandText = "/" + commandText
	}

	result, _, err := apiClient.ExecuteCommand(ctx, channelId, commandText)
	if err != nil {
		return fmt.Errorf("executing command: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(result)
		return nil
	}

	if result.Text != "" {
		printer.PrintInfo(result.Text)
	} else {
		printer.PrintSuccess("Command executed")
	}
	return nil
}

func slashListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	commands, _, err := apiClient.ListCommands(ctx, teamId, true)
	if err != nil {
		return fmt.Errorf("listing commands: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(commands)
		return nil
	}

	var rows [][]string
	for _, slashCommand := range commands {
		rows = append(rows, []string{
			"/" + slashCommand.Trigger,
			slashCommand.DisplayName,
			printer.Truncate(slashCommand.Description, 50),
			slashCommand.Id,
		})
	}

	if len(rows) == 0 {
		printer.PrintInfo("No custom commands")
		return nil
	}

	printer.PrintTable([]string{"TRIGGER", "NAME", "DESCRIPTION", "ID"}, rows)
	return nil
}

func init() {
	pluginCommand := &cobra.Command{
		Use:   "plugin",
		Short: "Plugin operations",
	}

	pluginListCommand := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE:  pluginListRun,
	}

	pluginCommand.AddCommand(pluginListCommand)
	rootCommand.AddCommand(pluginCommand)
}

func pluginListRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	plugins, _, err := apiClient.GetPlugins(ctx)
	if err != nil {
		return fmt.Errorf("listing plugins: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(plugins)
		return nil
	}

	var rows [][]string
	for _, plugin := range plugins.Active {
		rows = append(rows, []string{plugin.Manifest.Id, plugin.Manifest.Name, plugin.Manifest.Version, "active"})
	}
	for _, plugin := range plugins.Inactive {
		rows = append(rows, []string{plugin.Manifest.Id, plugin.Manifest.Name, plugin.Manifest.Version, "inactive"})
	}

	if len(rows) == 0 {
		printer.PrintInfo("No plugins installed")
		return nil
	}

	printer.PrintTable([]string{"ID", "NAME", "VERSION", "STATUS"}, rows)
	return nil
}
