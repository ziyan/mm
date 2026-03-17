package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	scheduledCommand := &cobra.Command{
		Use:   "scheduled",
		Short: "Scheduled post operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List your scheduled posts",
		RunE:  scheduledListRun,
	}

	createCommand := &cobra.Command{
		Use:   "create <channel> <time> <message>",
		Short: "Schedule a post (time format: 2006-01-02T15:04 or duration like 1h30m)",
		Args:  cobra.MinimumNArgs(3),
		RunE:  scheduledCreateRun,
	}
	createCommand.Flags().String("root-id", "", "Thread root post ID for a reply")

	deleteCommand := &cobra.Command{
		Use:   "delete <scheduled-post-id>",
		Short: "Delete a scheduled post",
		Args:  cobra.ExactArgs(1),
		RunE:  scheduledDeleteRun,
	}

	scheduledCommand.AddCommand(listCommand, createCommand, deleteCommand)
	rootCommand.AddCommand(scheduledCommand)
}

func scheduledListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	postsMap, _, err := apiClient.GetUserScheduledPosts(ctx, teamId, true)
	if err != nil {
		return fmt.Errorf("listing scheduled posts: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postsMap)
		return nil
	}

	var allPosts []*model.ScheduledPost
	for _, posts := range postsMap {
		allPosts = append(allPosts, posts...)
	}

	if len(allPosts) == 0 {
		printer.PrintInfo("No scheduled posts")
		return nil
	}

	var rows [][]string
	for _, post := range allPosts {
		channelName := post.ChannelId[:8]
		channel, _, err := apiClient.GetChannel(ctx, post.ChannelId)
		if err == nil {
			channelName = channel.DisplayName
		}
		rows = append(rows, []string{
			post.Id,
			channelName,
			printer.FormatTime(post.ScheduledAt),
			printer.Truncate(post.Message, 50),
		})
	}
	printer.PrintTable([]string{"ID", "CHANNEL", "SCHEDULED AT", "MESSAGE"}, rows)
	return nil
}

func scheduledCreateRun(command *cobra.Command, args []string) error {
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

	scheduledAt, err := parseScheduleTime(args[1])
	if err != nil {
		return err
	}

	message := strings.Join(args[2:], " ")
	rootId, _ := command.Flags().GetString("root-id")

	post := &model.ScheduledPost{
		Draft: model.Draft{
			ChannelId: channelId,
			Message:   message,
			RootId:    rootId,
		},
		ScheduledAt: scheduledAt,
	}

	created, _, err := apiClient.CreateScheduledPost(ctx, post)
	if err != nil {
		return fmt.Errorf("scheduling post: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(created)
		return nil
	}

	printer.PrintSuccess("Post scheduled for %s in %s", printer.FormatTime(scheduledAt), args[0])
	return nil
}

func scheduledDeleteRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, _, err = apiClient.DeleteScheduledPost(ctx, args[0])
	if err != nil {
		return fmt.Errorf("deleting scheduled post: %w", err)
	}

	printer.PrintSuccess("Deleted scheduled post %s", args[0])
	return nil
}

func parseScheduleTime(input string) (int64, error) {
	// Try duration first (e.g., "1h30m", "2h", "30m")
	duration, err := time.ParseDuration(input)
	if err == nil {
		return model.GetMillis() + duration.Milliseconds(), nil
	}

	// Try absolute datetime
	formats := []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"15:04",
	}

	for _, format := range formats {
		parsed, err := time.ParseInLocation(format, input, time.Local)
		if err == nil {
			if format == "15:04" {
				now := time.Now()
				parsed = time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.Local)
				if parsed.Before(now) {
					parsed = parsed.Add(24 * time.Hour)
				}
			}
			return parsed.UnixMilli(), nil
		}
	}

	return 0, fmt.Errorf("invalid time format %q (use: 2006-01-02T15:04, 15:04, or duration like 1h30m)", input)
}
