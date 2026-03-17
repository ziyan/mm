package commands

import (
	"context"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/config"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	teamCommand := &cobra.Command{
		Use:   "team",
		Short: "Manage teams",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List teams you belong to",
		RunE:  teamListRun,
	}

	switchCommand := &cobra.Command{
		Use:   "switch <team-name>",
		Short: "Set active team",
		Args:  cobra.ExactArgs(1),
		RunE:  teamSwitchRun,
	}

	infoCommand := &cobra.Command{
		Use:   "info [team-name]",
		Short: "Show team details",
		Args:  cobra.MaximumNArgs(1),
		RunE:  teamInfoRun,
	}

	membersCommand := &cobra.Command{
		Use:   "members [team-name]",
		Short: "List team members",
		Args:  cobra.MaximumNArgs(1),
		RunE:  teamMembersRun,
	}

	teamCommand.AddCommand(listCommand, switchCommand, infoCommand, membersCommand)
	rootCommand.AddCommand(teamCommand)
}

func teamListRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}

	teams, _, err := apiClient.GetTeamsForUser(ctx, currentUser.Id, "")
	if err != nil {
		return fmt.Errorf("listing teams: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(teams)
		return nil
	}

	configuration, _ := config.Load()
	server, _ := configuration.ActiveServer()

	var rows [][]string
	for _, team := range teams {
		active := ""
		if server != nil && team.Id == server.TeamID {
			active = "*"
		}
		rows = append(rows, []string{active, team.Name, team.DisplayName, team.Id})
	}
	printer.PrintTable([]string{"", "NAME", "DISPLAY NAME", "ID"}, rows)
	return nil
}

func teamSwitchRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	team, _, err := apiClient.GetTeamByName(ctx, args[0], "")
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	configuration, err := config.Load()
	if err != nil {
		return err
	}

	profile := configuration.Profiles[server.Name]
	profile.TeamID = team.Id
	profile.TeamName = team.Name
	configuration.Profiles[server.Name] = profile

	if err := configuration.Save(); err != nil {
		return err
	}

	printer.PrintSuccess("Switched to team %s (%s)", team.DisplayName, team.Name)
	return nil
}

func teamInfoRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	var teamName string
	if len(args) > 0 {
		teamName = args[0]
	} else if server.TeamName != "" {
		teamName = server.TeamName
	} else {
		return fmt.Errorf("specify a team name or set active team with: mm team switch <name>")
	}

	team, _, err := apiClient.GetTeamByName(ctx, teamName, "")
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(team)
		return nil
	}

	printer.PrintInfo("Name:         %s", team.Name)
	printer.PrintInfo("Display Name: %s", team.DisplayName)
	printer.PrintInfo("Description:  %s", team.Description)
	printer.PrintInfo("ID:           %s", team.Id)
	printer.PrintInfo("Type:         %s", team.Type)
	printer.PrintInfo("Email:        %s", team.Email)
	return nil
}

func teamMembersRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId := server.TeamID
	if len(args) > 0 {
		team, _, err := apiClient.GetTeamByName(ctx, args[0], "")
		if err != nil {
			return fmt.Errorf("team not found: %w", err)
		}
		teamId = team.Id
	}
	if teamId == "" {
		return fmt.Errorf("specify a team name or set active team with: mm team switch <name>")
	}

	members, _, err := apiClient.GetTeamMembers(ctx, teamId, 0, 200, "")
	if err != nil {
		return fmt.Errorf("listing members: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(members)
		return nil
	}

	userIds := make([]string, len(members))
	for index, member := range members {
		userIds[index] = member.UserId
	}
	users, _, err := apiClient.GetUsersByIds(ctx, userIds)
	if err != nil {
		return fmt.Errorf("fetching users: %w", err)
	}
	userById := make(map[string]*model.User)
	for _, user := range users {
		userById[user.Id] = user
	}

	var rows [][]string
	for _, member := range members {
		user, ok := userById[member.UserId]
		if !ok {
			continue
		}
		roles := member.Roles
		if roles == "" {
			roles = "-"
		}
		rows = append(rows, []string{user.Username, user.GetFullName(), roles})
	}
	printer.PrintTable([]string{"USERNAME", "NAME", "ROLES"}, rows)
	return nil
}
