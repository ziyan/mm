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
	savedCommand := &cobra.Command{
		Use:     "saved",
		Aliases: []string{"flagged"},
		Short:   "Saved/flagged post operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List saved posts",
		RunE:  savedListRun,
	}
	listCommand.Flags().String("channel", "", "Filter to a specific channel")
	listCommand.Flags().IntP("count", "n", 20, "Number of posts")

	addCommand := &cobra.Command{
		Use:   "add <post-id>",
		Short: "Save/flag a post",
		Args:  cobra.ExactArgs(1),
		RunE:  savedAddRun,
	}

	removeCommand := &cobra.Command{
		Use:   "remove <post-id>",
		Short: "Unsave/unflag a post",
		Args:  cobra.ExactArgs(1),
		RunE:  savedRemoveRun,
	}

	savedCommand.AddCommand(listCommand, addCommand, removeCommand)
	rootCommand.AddCommand(savedCommand)
}

func printPostList(apiClient *model.Client4, ctx context.Context, postList *model.PostList) {
	if len(postList.Order) == 0 {
		printer.PrintInfo("No posts")
		return
	}
	userCache := make(map[string]string)
	for index := len(postList.Order) - 1; index >= 0; index-- {
		post := postList.Posts[postList.Order[index]]
		fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
}

func savedListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	count, _ := command.Flags().GetInt("count")
	channelFilter, _ := command.Flags().GetString("channel")

	if channelFilter != "" {
		teamId, err := resolveTeamId(ctx, command, apiClient, server)
		if err != nil {
			return err
		}
		channelId, err := resolveChannelId(ctx, apiClient, teamId, channelFilter)
		if err != nil {
			return err
		}
		result, _, err := apiClient.GetFlaggedPostsForUserInChannel(ctx, currentUser.Id, channelId, 0, count)
		if err != nil {
			return fmt.Errorf("listing saved posts: %w", err)
		}
		if printer.JSONOutput {
			printer.PrintJSON(result)
			return nil
		}
		printPostList(apiClient, ctx, result)
	} else {
		result, _, err := apiClient.GetFlaggedPostsForUser(ctx, currentUser.Id, 0, count)
		if err != nil {
			return fmt.Errorf("listing saved posts: %w", err)
		}
		if printer.JSONOutput {
			printer.PrintJSON(result)
			return nil
		}
		printPostList(apiClient, ctx, result)
	}

	return nil
}

func savedAddRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	preferences := model.Preferences{
		{
			UserId:   currentUser.Id,
			Category: model.PreferenceCategoryFlaggedPost,
			Name:     args[0],
			Value:    "true",
		},
	}

	_, err = apiClient.UpdatePreferences(ctx, currentUser.Id, preferences)
	if err != nil {
		return fmt.Errorf("saving post: %w", err)
	}

	printer.PrintSuccess("Saved post %s", args[0])
	return nil
}

func savedRemoveRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	preferences := model.Preferences{
		{
			UserId:   currentUser.Id,
			Category: model.PreferenceCategoryFlaggedPost,
			Name:     args[0],
		},
	}

	_, err = apiClient.DeletePreferences(ctx, currentUser.Id, preferences)
	if err != nil {
		return fmt.Errorf("unsaving post: %w", err)
	}

	printer.PrintSuccess("Unsaved post %s", args[0])
	return nil
}
