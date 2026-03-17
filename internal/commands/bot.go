package commands

import (
	"context"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	botCommand := &cobra.Command{
		Use:   "bot",
		Short: "Bot operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List bots",
		RunE:  botListRun,
	}

	createCommand := &cobra.Command{
		Use:   "create <username>",
		Short: "Create a bot",
		Args:  cobra.ExactArgs(1),
		RunE:  botCreateRun,
	}
	createCommand.Flags().String("display-name", "", "Display name")
	createCommand.Flags().String("description", "", "Description")

	infoCommand := &cobra.Command{
		Use:   "info <bot-id>",
		Short: "Show bot details",
		Args:  cobra.ExactArgs(1),
		RunE:  botInfoRun,
	}

	disableCommand := &cobra.Command{
		Use:   "disable <bot-id>",
		Short: "Disable a bot",
		Args:  cobra.ExactArgs(1),
		RunE:  botDisableRun,
	}

	enableCommand := &cobra.Command{
		Use:   "enable <bot-id>",
		Short: "Enable a bot",
		Args:  cobra.ExactArgs(1),
		RunE:  botEnableRun,
	}

	botCommand.AddCommand(listCommand, createCommand, infoCommand, disableCommand, enableCommand)
	rootCommand.AddCommand(botCommand)
}

func botListRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	bots, _, err := apiClient.GetBots(ctx, 0, 200, "")
	if err != nil {
		return fmt.Errorf("listing bots: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(bots)
		return nil
	}

	var rows [][]string
	for _, bot := range bots {
		rows = append(rows, []string{bot.Username, bot.DisplayName, bot.Description, bot.UserId})
	}
	printer.PrintTable([]string{"USERNAME", "DISPLAY NAME", "DESCRIPTION", "USER ID"}, rows)
	return nil
}

func botCreateRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	displayName, _ := command.Flags().GetString("display-name")
	description, _ := command.Flags().GetString("description")

	if displayName == "" {
		displayName = args[0]
	}

	bot, _, err := apiClient.CreateBot(ctx, &model.Bot{
		Username:    args[0],
		DisplayName: displayName,
		Description: description,
	})
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(bot)
		return nil
	}

	printer.PrintSuccess("Created bot %s (user_id: %s)", bot.Username, bot.UserId)
	return nil
}

func botInfoRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	bot, _, err := apiClient.GetBot(ctx, args[0], "")
	if err != nil {
		return fmt.Errorf("bot not found: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(bot)
		return nil
	}

	printer.PrintInfo("Username:     %s", bot.Username)
	printer.PrintInfo("Display Name: %s", bot.DisplayName)
	printer.PrintInfo("Description:  %s", bot.Description)
	printer.PrintInfo("User ID:      %s", bot.UserId)
	printer.PrintInfo("Owner ID:     %s", bot.OwnerId)
	printer.PrintInfo("Created:      %s", printer.FormatTime(bot.CreateAt))
	return nil
}

func botDisableRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, _, err = apiClient.DisableBot(ctx, args[0])
	if err != nil {
		return fmt.Errorf("disabling bot: %w", err)
	}

	printer.PrintSuccess("Disabled bot %s", args[0])
	return nil
}

func botEnableRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, _, err = apiClient.EnableBot(ctx, args[0])
	if err != nil {
		return fmt.Errorf("enabling bot: %w", err)
	}

	printer.PrintSuccess("Enabled bot %s", args[0])
	return nil
}
