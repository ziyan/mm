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
	dmCommand := &cobra.Command{
		Use:   "dm",
		Short: "Direct messages",
	}

	sendCommand := &cobra.Command{
		Use:   "send <username> <message>",
		Short: "Send a direct message",
		Args:  cobra.MinimumNArgs(2),
		RunE:  dmSendRun,
	}

	readCommand := &cobra.Command{
		Use:   "read <username>",
		Short: "Read DM history with a user",
		Args:  cobra.ExactArgs(1),
		RunE:  dmReadRun,
	}
	readCommand.Flags().IntP("count", "n", 20, "Number of messages")

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List recent DM conversations",
		RunE:  dmListRun,
	}

	groupCommand := &cobra.Command{
		Use:   "group <username1,username2,...> <message>",
		Short: "Send a group message",
		Args:  cobra.MinimumNArgs(2),
		RunE:  dmGroupRun,
	}

	dmCommand.AddCommand(sendCommand, readCommand, listCommand, groupCommand)
	rootCommand.AddCommand(dmCommand)
}

func resolveUserId(ctx context.Context, apiClient *model.Client4, username string) (string, error) {
	user, _, err := apiClient.GetUserByUsername(ctx, username, "")
	if err != nil {
		return "", fmt.Errorf("user %q not found: %w", username, err)
	}
	return user.Id, nil
}

func dmSendRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	otherUserId, err := resolveUserId(ctx, apiClient, args[0])
	if err != nil {
		return err
	}

	channel, _, err := apiClient.CreateDirectChannel(ctx, currentUser.Id, otherUserId)
	if err != nil {
		return fmt.Errorf("creating DM channel: %w", err)
	}

	message := strings.Join(args[1:], " ")
	post, _, err := apiClient.CreatePost(ctx, &model.Post{
		ChannelId: channel.Id,
		Message:   message,
	})
	if err != nil {
		return fmt.Errorf("sending DM: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(post)
		return nil
	}

	printer.PrintSuccess("Sent DM to %s", args[0])
	return nil
}

func dmReadRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	otherUserId, err := resolveUserId(ctx, apiClient, args[0])
	if err != nil {
		return err
	}

	channel, _, err := apiClient.CreateDirectChannel(ctx, currentUser.Id, otherUserId)
	if err != nil {
		return fmt.Errorf("opening DM channel: %w", err)
	}

	count, _ := command.Flags().GetInt("count")
	postList, _, err := apiClient.GetPostsForChannel(ctx, channel.Id, 0, count, "", false, false)
	if err != nil {
		return fmt.Errorf("reading DMs: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postList)
		return nil
	}

	userCache := make(map[string]string)
	for index := len(postList.Order) - 1; index >= 0; index-- {
		post := postList.Posts[postList.Order[index]]
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

func dmListRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	channels, _, err := apiClient.GetChannelsForTeamForUser(ctx, "", currentUser.Id, false, "")
	if err != nil {
		return fmt.Errorf("listing channels: %w", err)
	}

	if printer.JSONOutput {
		var directMessages []*model.Channel
		for _, channel := range channels {
			if channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup {
				directMessages = append(directMessages, channel)
			}
		}
		printer.PrintJSON(directMessages)
		return nil
	}

	var rows [][]string
	for _, channel := range channels {
		if channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup {
			displayName := channel.DisplayName
			if displayName == "" {
				displayName = channel.Name
			}
			rows = append(rows, []string{
				displayName,
				printer.ChannelTypeName(string(channel.Type)),
				printer.FormatTime(channel.LastPostAt),
				channel.Id,
			})
		}
	}

	if len(rows) == 0 {
		printer.PrintInfo("No direct message conversations")
		return nil
	}

	printer.PrintTable([]string{"NAME", "TYPE", "LAST MSG", "ID"}, rows)
	return nil
}

func dmGroupRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	usernames := strings.Split(args[0], ",")
	userIds := []string{currentUser.Id}
	for _, username := range usernames {
		userId, err := resolveUserId(ctx, apiClient, strings.TrimSpace(username))
		if err != nil {
			return err
		}
		userIds = append(userIds, userId)
	}

	channel, _, err := apiClient.CreateGroupChannel(ctx, userIds)
	if err != nil {
		return fmt.Errorf("creating group channel: %w", err)
	}

	message := strings.Join(args[1:], " ")
	post, _, err := apiClient.CreatePost(ctx, &model.Post{
		ChannelId: channel.Id,
		Message:   message,
	})
	if err != nil {
		return fmt.Errorf("sending group message: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(post)
		return nil
	}

	printer.PrintSuccess("Sent group message to %s", args[0])
	return nil
}
