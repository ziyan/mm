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
	groupCommand := &cobra.Command{
		Use:   "group",
		Short: "Group operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List groups",
		RunE:  groupListRun,
	}
	listCommand.Flags().String("channel", "", "Filter groups by channel")
	listCommand.Flags().IntP("count", "n", 50, "Number of groups")

	membersCommand := &cobra.Command{
		Use:   "members <group-id>",
		Short: "List group members",
		Args:  cobra.ExactArgs(1),
		RunE:  groupMembersRun,
	}

	infoCommand := &cobra.Command{
		Use:   "info <group-id>",
		Short: "Show group details",
		Args:  cobra.ExactArgs(1),
		RunE:  groupInfoRun,
	}

	groupCommand.AddCommand(listCommand, membersCommand, infoCommand)
	rootCommand.AddCommand(groupCommand)
}

func groupListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

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
		groupsWithScheme, _, _, err := apiClient.GetGroupsByChannel(ctx, channelId, model.GroupSearchOpts{
			PageOpts: &model.PageOpts{Page: 0, PerPage: count},
		})
		if err != nil {
			return fmt.Errorf("listing groups: %w", err)
		}
		var groups []*model.Group
		for _, gws := range groupsWithScheme {
			groups = append(groups, &gws.Group)
		}
		return printGroups(groups)
	}

	groups, _, err := apiClient.GetGroups(ctx, model.GroupSearchOpts{
		PageOpts: &model.PageOpts{Page: 0, PerPage: count},
	})
	if err != nil {
		return fmt.Errorf("listing groups: %w", err)
	}
	return printGroups(groups)
}

func printGroups(groups []*model.Group) error {
	if printer.JSONOutput {
		printer.PrintJSON(groups)
		return nil
	}

	if len(groups) == 0 {
		printer.PrintInfo("No groups")
		return nil
	}

	var rows [][]string
	for _, group := range groups {
		displayName := ""
		if group.DisplayName != "" {
			displayName = group.DisplayName
		}
		source := string(group.Source)
		rows = append(rows, []string{group.Id, *group.Name, displayName, source})
	}
	printer.PrintTable([]string{"ID", "NAME", "DISPLAY NAME", "SOURCE"}, rows)
	return nil
}

func groupMembersRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	memberList, _, err := apiClient.GetGroupMembers(ctx, args[0])
	if err != nil {
		return fmt.Errorf("listing group members: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(memberList)
		return nil
	}

	var rows [][]string
	for _, user := range memberList.Members {
		rows = append(rows, []string{user.Username, user.GetFullName(), user.Email})
	}
	printer.PrintTable([]string{"USERNAME", "NAME", "EMAIL"}, rows)
	return nil
}

func groupInfoRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	group, _, err := apiClient.GetGroup(ctx, args[0], "")
	if err != nil {
		return fmt.Errorf("group not found: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(group)
		return nil
	}

	name := ""
	if group.Name != nil {
		name = *group.Name
	}
	printer.PrintInfo("ID:           %s", group.Id)
	printer.PrintInfo("Name:         %s", name)
	printer.PrintInfo("Display Name: %s", group.DisplayName)
	printer.PrintInfo("Source:       %s", group.Source)
	printer.PrintInfo("Member Count: %d", group.MemberCount)
	return nil
}
