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
	readCommand := &cobra.Command{
		Use:   "read <channel>",
		Short: "Mark a channel as read",
		Args:  cobra.ExactArgs(1),
		RunE:  channelReadRun,
	}

	notifyCommand := &cobra.Command{
		Use:   "notify <channel>",
		Short: "Update channel notification preferences",
		Args:  cobra.ExactArgs(1),
		RunE:  channelNotifyRun,
	}
	notifyCommand.Flags().String("desktop", "", "Desktop notifications (default, all, mention, none)")
	notifyCommand.Flags().String("push", "", "Push notifications (default, all, mention, none)")
	notifyCommand.Flags().String("email", "", "Email notifications (default, true, false)")
	notifyCommand.Flags().String("mark-unread", "", "Mark unread (all, mention)")

	favoriteCommand := &cobra.Command{
		Use:   "favorite <channel>",
		Short: "Mark a channel as favorite",
		Args:  cobra.ExactArgs(1),
		RunE:  channelFavoriteRun,
	}

	unfavoriteCommand := &cobra.Command{
		Use:   "unfavorite <channel>",
		Short: "Remove a channel from favorites",
		Args:  cobra.ExactArgs(1),
		RunE:  channelUnfavoriteRun,
	}

	categoriesCommand := &cobra.Command{
		Use:   "categories",
		Short: "List sidebar categories",
		RunE:  channelCategoriesRun,
	}

	for _, child := range rootCommand.Commands() {
		if child.Name() == "channel" {
			child.AddCommand(readCommand, notifyCommand, favoriteCommand, unfavoriteCommand, categoriesCommand)
			break
		}
	}
}

func channelReadRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	_, _, err = apiClient.ViewChannel(ctx, currentUser.Id, &model.ChannelView{
		ChannelId: channelId,
	})
	if err != nil {
		return fmt.Errorf("marking channel as read: %w", err)
	}

	printer.PrintSuccess("Marked %s as read", args[0])
	return nil
}

func channelNotifyRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	props := map[string]string{}

	if desktop, _ := command.Flags().GetString("desktop"); desktop != "" {
		props[model.DesktopNotifyProp] = desktop
	}
	if push, _ := command.Flags().GetString("push"); push != "" {
		props[model.PushNotifyProp] = push
	}
	if email, _ := command.Flags().GetString("email"); email != "" {
		props[model.EmailNotifyProp] = email
	}
	if markUnread, _ := command.Flags().GetString("mark-unread"); markUnread != "" {
		props[model.MarkUnreadNotifyProp] = markUnread
	}

	if len(props) == 0 {
		// Show current settings
		member, _, err := apiClient.GetChannelMember(ctx, channelId, currentUser.Id, "")
		if err != nil {
			return fmt.Errorf("getting channel member: %w", err)
		}
		if printer.JSONOutput {
			printer.PrintJSON(member.NotifyProps)
			return nil
		}
		for key, value := range member.NotifyProps {
			printer.PrintInfo("%-15s %s", key, value)
		}
		return nil
	}

	_, err = apiClient.UpdateChannelNotifyProps(ctx, channelId, currentUser.Id, props)
	if err != nil {
		return fmt.Errorf("updating notification settings: %w", err)
	}

	printer.PrintSuccess("Updated notification settings for %s", args[0])
	return nil
}

func channelFavoriteRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	preferences := model.Preferences{
		{
			UserId:   currentUser.Id,
			Category: model.PreferenceCategoryFavoriteChannel,
			Name:     channelId,
			Value:    "true",
		},
	}

	_, err = apiClient.UpdatePreferences(ctx, currentUser.Id, preferences)
	if err != nil {
		return fmt.Errorf("favoriting channel: %w", err)
	}

	printer.PrintSuccess("Favorited channel %s", args[0])
	return nil
}

func channelUnfavoriteRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	preferences := model.Preferences{
		{
			UserId:   currentUser.Id,
			Category: model.PreferenceCategoryFavoriteChannel,
			Name:     channelId,
		},
	}

	_, err = apiClient.DeletePreferences(ctx, currentUser.Id, preferences)
	if err != nil {
		return fmt.Errorf("unfavoriting channel: %w", err)
	}

	printer.PrintSuccess("Unfavorited channel %s", args[0])
	return nil
}

func channelCategoriesRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	categories, _, err := apiClient.GetSidebarCategoriesForTeamForUser(ctx, currentUser.Id, teamId, "")
	if err != nil {
		return fmt.Errorf("listing categories: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(categories)
		return nil
	}

	for _, category := range categories.Categories {
		printer.PrintInfo("%s (%s) - %d channels", category.DisplayName, category.Type, len(category.Channels))
	}
	return nil
}
