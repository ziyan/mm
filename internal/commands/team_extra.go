package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	inviteCommand := &cobra.Command{
		Use:   "invite <email>[,<email>...]",
		Short: "Invite users to the team by email",
		Args:  cobra.ExactArgs(1),
		RunE:  teamInviteRun,
	}

	for _, child := range rootCommand.Commands() {
		if child.Name() == "team" {
			child.AddCommand(inviteCommand)
			break
		}
	}
}

func teamInviteRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	emails := strings.Split(args[0], ",")
	for index := range emails {
		emails[index] = strings.TrimSpace(emails[index])
	}

	_, err = apiClient.InviteUsersToTeam(ctx, teamId, emails)
	if err != nil {
		return fmt.Errorf("inviting users: %w", err)
	}

	printer.PrintSuccess("Invited %d user(s) to the team", len(emails))
	return nil
}
