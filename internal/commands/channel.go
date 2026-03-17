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
	channelCommand := &cobra.Command{
		Use:     "channel",
		Aliases: []string{"ch"},
		Short:   "Manage channels",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List channels in active team",
		RunE:  channelListRun,
	}
	listCommand.Flags().BoolP("all", "a", false, "Include channels you haven't joined")

	joinCommand := &cobra.Command{
		Use:   "join <channel-name>",
		Short: "Join a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  channelJoinRun,
	}

	leaveCommand := &cobra.Command{
		Use:   "leave <channel-name>",
		Short: "Leave a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  channelLeaveRun,
	}

	createCommand := &cobra.Command{
		Use:   "create <channel-name>",
		Short: "Create a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  channelCreateRun,
	}
	createCommand.Flags().String("display-name", "", "Display name")
	createCommand.Flags().String("purpose", "", "Channel purpose")
	createCommand.Flags().String("header", "", "Channel header")
	createCommand.Flags().BoolP("private", "p", false, "Create as private channel")

	infoCommand := &cobra.Command{
		Use:   "info <channel-name>",
		Short: "Show channel details",
		Args:  cobra.ExactArgs(1),
		RunE:  channelInfoRun,
	}

	membersCommand := &cobra.Command{
		Use:   "members <channel-name>",
		Short: "List channel members",
		Args:  cobra.ExactArgs(1),
		RunE:  channelMembersRun,
	}

	archiveCommand := &cobra.Command{
		Use:   "archive <channel-name>",
		Short: "Archive a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  channelArchiveRun,
	}

	unreadCommand := &cobra.Command{
		Use:   "unread",
		Short: "List channels with unread messages",
		RunE:  channelUnreadRun,
	}
	unreadCommand.Flags().BoolP("mentions", "m", false, "Only show channels with mentions")

	channelCommand.AddCommand(listCommand, joinCommand, leaveCommand, createCommand, infoCommand, membersCommand, archiveCommand, unreadCommand)
	rootCommand.AddCommand(channelCommand)
}

func resolveChannelId(ctx context.Context, apiClient *model.Client4, teamId, channelName string) (string, error) {
	channel, _, err := apiClient.GetChannelByName(ctx, channelName, teamId, "")
	if err != nil {
		return "", fmt.Errorf("channel %q not found: %w", channelName, err)
	}
	return channel.Id, nil
}

func channelListRun(command *cobra.Command, args []string) error {
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

	showAll, _ := command.Flags().GetBool("all")

	var channels []*model.Channel
	if showAll {
		channels, _, err = apiClient.SearchChannels(ctx, teamId, &model.ChannelSearch{Term: ""})
	} else {
		channels, _, err = apiClient.GetChannelsForTeamForUser(ctx, teamId, currentUser.Id, false, "")
	}
	if err != nil {
		return fmt.Errorf("listing channels: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(channels)
		return nil
	}

	var rows [][]string
	for _, channel := range channels {
		rows = append(rows, []string{
			channel.Name,
			channel.DisplayName,
			printer.ChannelTypeName(string(channel.Type)),
			channel.Id,
		})
	}
	printer.PrintTable([]string{"NAME", "DISPLAY NAME", "TYPE", "ID"}, rows)
	return nil
}

func channelJoinRun(command *cobra.Command, args []string) error {
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

	_, _, err = apiClient.AddChannelMember(ctx, channelId, currentUser.Id)
	if err != nil {
		return fmt.Errorf("joining channel: %w", err)
	}

	printer.PrintSuccess("Joined channel %s", args[0])
	return nil
}

func channelLeaveRun(command *cobra.Command, args []string) error {
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

	_, err = apiClient.RemoveUserFromChannel(ctx, channelId, currentUser.Id)
	if err != nil {
		return fmt.Errorf("leaving channel: %w", err)
	}

	printer.PrintSuccess("Left channel %s", args[0])
	return nil
}

func channelCreateRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	displayName, _ := command.Flags().GetString("display-name")
	purpose, _ := command.Flags().GetString("purpose")
	header, _ := command.Flags().GetString("header")
	private, _ := command.Flags().GetBool("private")

	if displayName == "" {
		displayName = args[0]
	}

	channelType := model.ChannelTypeOpen
	if private {
		channelType = model.ChannelTypePrivate
	}

	channel, _, err := apiClient.CreateChannel(ctx, &model.Channel{
		TeamId:      teamId,
		Name:        args[0],
		DisplayName: displayName,
		Purpose:     purpose,
		Header:      header,
		Type:        channelType,
	})
	if err != nil {
		return fmt.Errorf("creating channel: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(channel)
		return nil
	}

	printer.PrintSuccess("Created channel %s (%s)", channel.Name, channel.Id)
	return nil
}

func channelInfoRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	channel, _, err := apiClient.GetChannelByName(ctx, args[0], teamId, "")
	if err != nil {
		return fmt.Errorf("channel not found: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(channel)
		return nil
	}

	printer.PrintInfo("Name:         %s", channel.Name)
	printer.PrintInfo("Display Name: %s", channel.DisplayName)
	printer.PrintInfo("Type:         %s", printer.ChannelTypeName(string(channel.Type)))
	printer.PrintInfo("Purpose:      %s", channel.Purpose)
	printer.PrintInfo("Header:       %s", channel.Header)
	printer.PrintInfo("ID:           %s", channel.Id)
	printer.PrintInfo("Created:      %s", printer.FormatTime(channel.CreateAt))
	return nil
}

func channelMembersRun(command *cobra.Command, args []string) error {
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

	members, _, err := apiClient.GetChannelMembers(ctx, channelId, 0, 200, "")
	if err != nil {
		return fmt.Errorf("listing members: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(members)
		return nil
	}

	var rows [][]string
	for _, member := range members {
		user, _, err := apiClient.GetUser(ctx, member.UserId, "")
		if err != nil {
			continue
		}
		rows = append(rows, []string{user.Username, user.GetFullName(), member.Roles})
	}
	printer.PrintTable([]string{"USERNAME", "NAME", "ROLES"}, rows)
	return nil
}

func channelArchiveRun(command *cobra.Command, args []string) error {
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

	_, err = apiClient.DeleteChannel(ctx, channelId)
	if err != nil {
		return fmt.Errorf("archiving channel: %w", err)
	}

	printer.PrintSuccess("Archived channel %s", args[0])
	return nil
}

func channelUnreadRun(command *cobra.Command, args []string) error {
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

	channels, _, err := apiClient.GetChannelsForTeamForUser(ctx, teamId, currentUser.Id, false, "")
	if err != nil {
		return err
	}

	// Fetch all channel memberships in a single call instead of per-channel
	members, _, err := apiClient.GetChannelMembersForUser(ctx, currentUser.Id, teamId, "")
	if err != nil {
		return err
	}

	memberByChannel := make(map[string]*model.ChannelMember)
	for index := range members {
		memberByChannel[members[index].ChannelId] = &members[index]
	}

	channelNameById := make(map[string]string)
	for _, channel := range channels {
		channelNameById[channel.Id] = channel.DisplayName
	}

	type unreadEntry struct {
		name         string
		id           string
		mentionCount int64
		messageCount int64
	}

	mentionsOnly, _ := command.Flags().GetBool("mentions")

	var unreadEntries []unreadEntry
	for _, channel := range channels {
		member, ok := memberByChannel[channel.Id]
		if !ok {
			continue
		}
		unreadCount := channel.TotalMsgCount - member.MsgCount
		if mentionsOnly {
			if member.MentionCount == 0 {
				continue
			}
		} else if unreadCount <= 0 && member.MentionCount == 0 {
			continue
		}
		if unreadCount > 0 || member.MentionCount > 0 {
			unreadEntries = append(unreadEntries, unreadEntry{
				name:         channel.DisplayName,
				id:           channel.Id,
				mentionCount: member.MentionCount,
				messageCount: unreadCount,
			})
		}
	}

	if printer.JSONOutput {
		type unreadJSON struct {
			Name         string `json:"name"`
			ID           string `json:"id"`
			MentionCount int64  `json:"mention_count"`
			MessageCount int64  `json:"message_count"`
		}
		var result []unreadJSON
		for _, entry := range unreadEntries {
			result = append(result, unreadJSON{
				Name:         entry.name,
				ID:           entry.id,
				MentionCount: entry.mentionCount,
				MessageCount: entry.messageCount,
			})
		}
		printer.PrintJSON(result)
		return nil
	}

	if len(unreadEntries) == 0 {
		printer.PrintInfo("No unread channels")
		return nil
	}

	var rows [][]string
	for _, entry := range unreadEntries {
		mention := ""
		if entry.mentionCount > 0 {
			mention = fmt.Sprintf("@%d", entry.mentionCount)
		}
		rows = append(rows, []string{entry.name, fmt.Sprintf("%d", entry.messageCount), mention})
	}
	printer.PrintTable([]string{"CHANNEL", "UNREAD", "MENTIONS"}, rows)
	return nil
}
