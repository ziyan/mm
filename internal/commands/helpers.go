package commands

import (
	"context"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/config"
)

// resolveTeamId returns the team ID from the --team flag, the active profile, or an error.
func resolveTeamId(ctx context.Context, command *cobra.Command, apiClient *model.Client4, server *config.ServerProfile) (string, error) {
	teamOverride, _ := command.Flags().GetString("team")
	if teamOverride != "" {
		team, _, err := apiClient.GetTeamByName(ctx, teamOverride, "")
		if err != nil {
			return "", fmt.Errorf("team %q not found: %w", teamOverride, err)
		}
		return team.Id, nil
	}
	if server.TeamID != "" {
		return server.TeamID, nil
	}
	return "", fmt.Errorf("no active team set. Use --team <name> or run: mm team switch <name>")
}
