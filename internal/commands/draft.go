package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	draftCommand := &cobra.Command{
		Use:   "draft",
		Short: "Message draft operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List your drafts",
		RunE:  draftListRun,
	}

	createCommand := &cobra.Command{
		Use:   "create <channel> <message>",
		Short: "Create or update a draft",
		Args:  cobra.MinimumNArgs(2),
		RunE:  draftCreateRun,
	}
	createCommand.Flags().String("root-id", "", "Thread root post ID for a reply draft")

	deleteCommand := &cobra.Command{
		Use:   "delete <channel>",
		Short: "Delete a draft for a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  draftDeleteRun,
	}

	draftCommand.AddCommand(listCommand, createCommand, deleteCommand)
	rootCommand.AddCommand(draftCommand)
}

func draftListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	drafts, _, err := apiClient.GetDrafts(ctx, currentUser.Id, teamId)
	if err != nil {
		return fmt.Errorf("listing drafts: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(drafts)
		return nil
	}

	if len(drafts) == 0 {
		printer.PrintInfo("No drafts")
		return nil
	}

	var rows [][]string
	for _, draft := range drafts {
		channelName := draft.ChannelId[:8]
		channel, _, err := apiClient.GetChannel(ctx, draft.ChannelId)
		if err == nil {
			channelName = channel.DisplayName
		}
		threadIndicator := ""
		if draft.RootId != "" {
			threadIndicator = "reply"
		}
		rows = append(rows, []string{
			channelName,
			threadIndicator,
			printer.Truncate(draft.Message, 60),
			printer.FormatTime(draft.UpdateAt),
		})
	}
	printer.PrintTable([]string{"CHANNEL", "TYPE", "MESSAGE", "UPDATED"}, rows)
	return nil
}

func draftCreateRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	message := strings.Join(args[1:], " ")
	rootId, _ := command.Flags().GetString("root-id")

	draft := &model.Draft{
		UserId:    currentUser.Id,
		ChannelId: channelId,
		Message:   message,
		RootId:    rootId,
	}

	_, _, err = apiClient.UpsertDraft(ctx, draft)
	if err != nil {
		return fmt.Errorf("saving draft: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(draft)
		return nil
	}

	printer.PrintSuccess("Draft saved for channel %s", args[0])
	return nil
}

func draftDeleteRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	_, _, err = apiClient.DeleteDraft(ctx, currentUser.Id, channelId, "")
	if err != nil {
		return fmt.Errorf("deleting draft: %w", err)
	}

	printer.PrintSuccess("Draft deleted for channel %s", args[0])
	return nil
}
