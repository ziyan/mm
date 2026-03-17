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
	bookmarkCommand := &cobra.Command{
		Use:   "bookmark",
		Short: "Channel bookmark operations",
	}

	listCommand := &cobra.Command{
		Use:   "list <channel>",
		Short: "List bookmarks in a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  bookmarkListRun,
	}

	addCommand := &cobra.Command{
		Use:   "add <channel> <display-name> <link-url>",
		Short: "Add a link bookmark to a channel",
		Args:  cobra.ExactArgs(3),
		RunE:  bookmarkAddRun,
	}
	addCommand.Flags().String("emoji", "", "Emoji for the bookmark")

	deleteCommand := &cobra.Command{
		Use:   "delete <channel> <bookmark-id>",
		Short: "Delete a bookmark",
		Args:  cobra.ExactArgs(2),
		RunE:  bookmarkDeleteRun,
	}

	bookmarkCommand.AddCommand(listCommand, addCommand, deleteCommand)
	rootCommand.AddCommand(bookmarkCommand)
}

func bookmarkListRun(command *cobra.Command, args []string) error {
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

	bookmarks, _, err := apiClient.ListChannelBookmarksForChannel(ctx, channelId, model.GetMillis())
	if err != nil {
		return fmt.Errorf("listing bookmarks: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(bookmarks)
		return nil
	}

	if len(bookmarks) == 0 {
		printer.PrintInfo("No bookmarks")
		return nil
	}

	var rows [][]string
	for _, bookmark := range bookmarks {
		linkUrl := ""
		if bookmark.LinkUrl != "" {
			linkUrl = bookmark.LinkUrl
		}
		rows = append(rows, []string{
			bookmark.DisplayName,
			string(bookmark.Type),
			linkUrl,
			bookmark.Id,
		})
	}
	printer.PrintTable([]string{"NAME", "TYPE", "URL", "ID"}, rows)
	return nil
}

func bookmarkAddRun(command *cobra.Command, args []string) error {
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

	emoji, _ := command.Flags().GetString("emoji")

	bookmark := &model.ChannelBookmark{
		ChannelId:   channelId,
		DisplayName: args[1],
		LinkUrl:     args[2],
		Type:        model.ChannelBookmarkLink,
		Emoji:       emoji,
	}

	created, _, err := apiClient.CreateChannelBookmark(ctx, bookmark)
	if err != nil {
		return fmt.Errorf("creating bookmark: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(created)
		return nil
	}

	printer.PrintSuccess("Added bookmark %q to %s", args[1], args[0])
	return nil
}

func bookmarkDeleteRun(command *cobra.Command, args []string) error {
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

	_, _, err = apiClient.DeleteChannelBookmark(ctx, channelId, args[1])
	if err != nil {
		return fmt.Errorf("deleting bookmark: %w", err)
	}

	printer.PrintSuccess("Deleted bookmark %s", args[1])
	return nil
}
