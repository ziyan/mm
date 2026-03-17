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
	userCommand := &cobra.Command{
		Use:   "user",
		Short: "User operations",
	}

	meCommand := &cobra.Command{
		Use:   "me",
		Short: "Show your profile",
		RunE:  userMeRun,
	}

	infoCommand := &cobra.Command{
		Use:   "info <username>",
		Short: "Show user profile",
		Args:  cobra.ExactArgs(1),
		RunE:  userInfoRun,
	}

	statusCommand := &cobra.Command{
		Use:   "status [online|away|dnd|offline]",
		Short: "Get or set your status",
		Args:  cobra.MaximumNArgs(1),
		RunE:  userStatusRun,
	}
	statusCommand.Flags().String("message", "", "Custom status text")
	statusCommand.Flags().String("emoji", "", "Custom status emoji")

	searchCommand := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for users",
		Args:  cobra.ExactArgs(1),
		RunE:  userSearchRun,
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List users in the team",
		RunE:  userListRun,
	}
	listCommand.Flags().IntP("count", "n", 50, "Number of users")

	userCommand.AddCommand(meCommand, infoCommand, statusCommand, searchCommand, listCommand)
	rootCommand.AddCommand(userCommand)
}

func printUserProfile(user *model.User) {
	printer.PrintInfo("Username:  %s", user.Username)
	printer.PrintInfo("Name:      %s", user.GetFullName())
	printer.PrintInfo("Email:     %s", user.Email)
	printer.PrintInfo("ID:        %s", user.Id)
	printer.PrintInfo("Position:  %s", user.Position)
	printer.PrintInfo("Locale:    %s", user.Locale)
	printer.PrintInfo("Roles:     %s", user.Roles)
	printer.PrintInfo("Created:   %s", printer.FormatTime(user.CreateAt))
	if user.DeleteAt > 0 {
		printer.PrintInfo("Deleted:   %s", printer.FormatTime(user.DeleteAt))
	}
}

func userMeRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return fmt.Errorf("getting profile: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(currentUser)
		return nil
	}

	printUserProfile(currentUser)
	return nil
}

func userInfoRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	user, _, err := apiClient.GetUserByUsername(ctx, args[0], "")
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(user)
		return nil
	}

	printUserProfile(user)

	status, _, err := apiClient.GetUserStatus(ctx, user.Id, "")
	if err == nil {
		printer.PrintInfo("Status:    %s", status.Status)
		if status.Manual {
			printer.PrintInfo("           (manual)")
		}
	}

	return nil
}

func userStatusRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	customMessage, _ := command.Flags().GetString("message")
	emoji, _ := command.Flags().GetString("emoji")

	if customMessage != "" || emoji != "" {
		customStatus := &model.CustomStatus{
			Text:  customMessage,
			Emoji: emoji,
		}
		_, _, err := apiClient.UpdateUserCustomStatus(ctx, currentUser.Id, customStatus)
		if err != nil {
			return fmt.Errorf("setting custom status: %w", err)
		}
		printer.PrintSuccess("Custom status updated")
	}

	if len(args) == 0 {
		status, _, err := apiClient.GetUserStatus(ctx, currentUser.Id, "")
		if err != nil {
			return fmt.Errorf("getting status: %w", err)
		}
		if printer.JSONOutput {
			printer.PrintJSON(status)
			return nil
		}
		printer.PrintInfo("Status: %s", status.Status)
		return nil
	}

	statusValue := args[0]
	switch statusValue {
	case "online", "away", "dnd", "offline":
	default:
		return fmt.Errorf("invalid status: %s (use: online, away, dnd, offline)", statusValue)
	}

	updatedStatus, _, err := apiClient.UpdateUserStatus(ctx, currentUser.Id, &model.Status{
		UserId: currentUser.Id,
		Status: statusValue,
		Manual: true,
	})
	if err != nil {
		return fmt.Errorf("setting status: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(updatedStatus)
		return nil
	}

	printer.PrintSuccess("Status set to %s", statusValue)
	return nil
}

func userSearchRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	users, _, err := apiClient.SearchUsers(ctx, &model.UserSearch{
		Term: args[0],
	})
	if err != nil {
		return fmt.Errorf("searching users: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(users)
		return nil
	}

	var rows [][]string
	for _, user := range users {
		rows = append(rows, []string{user.Username, user.GetFullName(), user.Email, user.Id})
	}
	printer.PrintTable([]string{"USERNAME", "NAME", "EMAIL", "ID"}, rows)
	return nil
}

func userListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	count, _ := command.Flags().GetInt("count")

	var users []*model.User
	if server.TeamID != "" {
		members, _, err := apiClient.GetTeamMembers(ctx, server.TeamID, 0, count, "")
		if err != nil {
			return fmt.Errorf("listing team members: %w", err)
		}
		userIds := make([]string, len(members))
		for index, member := range members {
			userIds[index] = member.UserId
		}
		users, _, err = apiClient.GetUsersByIds(ctx, userIds)
		if err != nil {
			return fmt.Errorf("fetching users: %w", err)
		}
	} else {
		var err error
		users, _, err = apiClient.GetUsers(ctx, 0, count, "")
		if err != nil {
			return fmt.Errorf("listing users: %w", err)
		}
	}

	if printer.JSONOutput {
		printer.PrintJSON(users)
		return nil
	}

	var rows [][]string
	for _, user := range users {
		rows = append(rows, []string{user.Username, user.GetFullName(), user.Email, user.Roles})
	}
	printer.PrintTable([]string{"USERNAME", "NAME", "EMAIL", "ROLES"}, rows)
	return nil
}
