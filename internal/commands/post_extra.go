package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	historyCommand := &cobra.Command{
		Use:   "history <post-id>",
		Short: "Show edit history of a post",
		Args:  cobra.ExactArgs(1),
	}
	historyCommand.RunE = postHistoryRun

	remindCommand := &cobra.Command{
		Use:   "remind <post-id> <duration>",
		Short: "Set a reminder for a post (e.g., 30m, 1h, 24h)",
		Args:  cobra.ExactArgs(2),
	}
	remindCommand.RunE = postRemindRun

	// Find the post command and add subcommands
	for _, child := range rootCommand.Commands() {
		if child.Name() == "post" {
			child.AddCommand(historyCommand, remindCommand)
			break
		}
	}
}

func postHistoryRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	history, _, err := apiClient.GetEditHistoryForPost(ctx, postId)
	if err != nil {
		return fmt.Errorf("getting edit history: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(history)
		return nil
	}

	if len(history) == 0 {
		printer.PrintInfo("No edit history")
		return nil
	}

	userCache := make(map[string]string)
	for _, post := range history {
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

func postRemindRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	postId := normalizePostId(args[0])

	duration, err := time.ParseDuration(args[1])
	if err != nil {
		return fmt.Errorf("invalid duration %q (use: 30m, 1h, 24h)", args[1])
	}

	targetTime := time.Now().Add(duration).Unix()

	_, err = apiClient.SetPostReminder(ctx, &model.PostReminder{
		PostId:     postId,
		UserId:     currentUser.Id,
		TargetTime: targetTime,
	})
	if err != nil {
		return fmt.Errorf("setting reminder: %w", err)
	}

	printer.PrintSuccess("Reminder set for post %s in %s", args[0], args[1])
	return nil
}
