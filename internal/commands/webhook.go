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
	webhookCommand := &cobra.Command{
		Use:   "webhook",
		Short: "Webhook operations",
	}

	listIncomingCommand := &cobra.Command{
		Use:   "list-incoming",
		Short: "List incoming webhooks",
		RunE:  webhookListIncomingRun,
	}

	listOutgoingCommand := &cobra.Command{
		Use:   "list-outgoing",
		Short: "List outgoing webhooks",
		RunE:  webhookListOutgoingRun,
	}

	createIncomingCommand := &cobra.Command{
		Use:   "create-incoming <channel>",
		Short: "Create incoming webhook",
		Args:  cobra.ExactArgs(1),
		RunE:  webhookCreateIncomingRun,
	}
	createIncomingCommand.Flags().String("display-name", "", "Display name (required)")
	createIncomingCommand.Flags().String("description", "", "Description")
	createIncomingCommand.MarkFlagRequired("display-name")

	createOutgoingCommand := &cobra.Command{
		Use:   "create-outgoing <channel>",
		Short: "Create outgoing webhook",
		Args:  cobra.ExactArgs(1),
		RunE:  webhookCreateOutgoingRun,
	}
	createOutgoingCommand.Flags().String("display-name", "", "Display name (required)")
	createOutgoingCommand.Flags().String("description", "", "Description")
	createOutgoingCommand.Flags().StringArray("trigger", nil, "Trigger words")
	createOutgoingCommand.Flags().StringArray("url", nil, "Callback URLs")
	createOutgoingCommand.MarkFlagRequired("display-name")
	createOutgoingCommand.MarkFlagRequired("url")

	deleteCommand := &cobra.Command{
		Use:   "delete <webhook-id>",
		Short: "Delete a webhook",
		Args:  cobra.ExactArgs(1),
		RunE:  webhookDeleteRun,
	}
	deleteCommand.Flags().Bool("outgoing", false, "Delete outgoing webhook (default: incoming)")

	webhookCommand.AddCommand(listIncomingCommand, listOutgoingCommand, createIncomingCommand, createOutgoingCommand, deleteCommand)
	rootCommand.AddCommand(webhookCommand)
}

func webhookListIncomingRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	hooks, _, err := apiClient.GetIncomingWebhooksForTeam(ctx, teamId, 0, 200, "")
	if err != nil {
		return fmt.Errorf("listing webhooks: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(hooks)
		return nil
	}

	var rows [][]string
	for _, hook := range hooks {
		rows = append(rows, []string{hook.DisplayName, hook.ChannelId, hook.Id})
	}
	printer.PrintTable([]string{"NAME", "CHANNEL ID", "ID"}, rows)
	return nil
}

func webhookListOutgoingRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	hooks, _, err := apiClient.GetOutgoingWebhooksForTeam(ctx, teamId, 0, 200, "")
	if err != nil {
		return fmt.Errorf("listing webhooks: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(hooks)
		return nil
	}

	var rows [][]string
	for _, hook := range hooks {
		rows = append(rows, []string{hook.DisplayName, hook.ChannelId, hook.Id})
	}
	printer.PrintTable([]string{"NAME", "CHANNEL ID", "ID"}, rows)
	return nil
}

func webhookCreateIncomingRun(command *cobra.Command, args []string) error {
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

	displayName, _ := command.Flags().GetString("display-name")
	description, _ := command.Flags().GetString("description")

	hook, _, err := apiClient.CreateIncomingWebhook(ctx, &model.IncomingWebhook{
		ChannelId:   channelId,
		DisplayName: displayName,
		Description: description,
		TeamId:      teamId,
	})
	if err != nil {
		return fmt.Errorf("creating webhook: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(hook)
		return nil
	}

	printer.PrintSuccess("Created incoming webhook %s (%s)", hook.DisplayName, hook.Id)
	return nil
}

func webhookCreateOutgoingRun(command *cobra.Command, args []string) error {
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

	displayName, _ := command.Flags().GetString("display-name")
	description, _ := command.Flags().GetString("description")
	triggerWords, _ := command.Flags().GetStringArray("trigger")
	callbackUrls, _ := command.Flags().GetStringArray("url")

	hook, _, err := apiClient.CreateOutgoingWebhook(ctx, &model.OutgoingWebhook{
		ChannelId:    channelId,
		TeamId:       teamId,
		DisplayName:  displayName,
		Description:  description,
		TriggerWords: triggerWords,
		CallbackURLs: callbackUrls,
	})
	if err != nil {
		return fmt.Errorf("creating webhook: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(hook)
		return nil
	}

	printer.PrintSuccess("Created outgoing webhook %s (%s)", hook.DisplayName, hook.Id)
	return nil
}

func webhookDeleteRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	outgoing, _ := command.Flags().GetBool("outgoing")

	if outgoing {
		_, err = apiClient.DeleteOutgoingWebhook(ctx, args[0])
	} else {
		_, err = apiClient.DeleteIncomingWebhook(ctx, args[0])
	}
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}

	printer.PrintSuccess("Deleted webhook %s", args[0])
	return nil
}
