package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	sessionsCommand := &cobra.Command{
		Use:   "sessions",
		Short: "List active sessions",
		RunE:  authSessionsRun,
	}

	revokeSessionCommand := &cobra.Command{
		Use:   "revoke-session <session-id>",
		Short: "Revoke a specific session",
		Args:  cobra.ExactArgs(1),
		RunE:  authRevokeSessionRun,
	}

	revokeAllCommand := &cobra.Command{
		Use:   "revoke-all",
		Short: "Revoke all sessions",
		RunE:  authRevokeAllRun,
	}

	tokenCreateCommand := &cobra.Command{
		Use:   "token-create <description>",
		Short: "Create a personal access token",
		Args:  cobra.ExactArgs(1),
		RunE:  authTokenCreateRun,
	}

	tokenListCommand := &cobra.Command{
		Use:   "token-list",
		Short: "List your personal access tokens",
		RunE:  authTokenListRun,
	}

	tokenRevokeCommand := &cobra.Command{
		Use:   "token-revoke <token-id>",
		Short: "Revoke a personal access token",
		Args:  cobra.ExactArgs(1),
		RunE:  authTokenRevokeRun,
	}

	for _, child := range rootCommand.Commands() {
		if child.Name() == "auth" {
			child.AddCommand(sessionsCommand, revokeSessionCommand, revokeAllCommand, tokenCreateCommand, tokenListCommand, tokenRevokeCommand)
			break
		}
	}
}

func authSessionsRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	sessions, _, err := apiClient.GetSessions(ctx, currentUser.Id, "")
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(sessions)
		return nil
	}

	var rows [][]string
	for _, session := range sessions {
		deviceInfo := session.DeviceId
		if deviceInfo == "" {
			deviceInfo = "-"
		}
		rows = append(rows, []string{
			session.Id,
			printer.Truncate(session.Props["os"]+" "+session.Props["browser"], 30),
			deviceInfo,
			printer.FormatTime(session.CreateAt),
			printer.FormatTime(session.ExpiresAt),
		})
	}
	printer.PrintTable([]string{"ID", "CLIENT", "DEVICE", "CREATED", "EXPIRES"}, rows)
	return nil
}

func authRevokeSessionRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, err = apiClient.RevokeSession(ctx, "me", args[0])
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}

	printer.PrintSuccess("Revoked session %s", args[0])
	return nil
}

func authRevokeAllRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	_, err = apiClient.RevokeAllSessions(ctx, currentUser.Id)
	if err != nil {
		return fmt.Errorf("revoking all sessions: %w", err)
	}

	printer.PrintSuccess("Revoked all sessions")
	return nil
}

func authTokenCreateRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	token, _, err := apiClient.CreateUserAccessToken(ctx, currentUser.Id, args[0])
	if err != nil {
		return fmt.Errorf("creating token: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(token)
		return nil
	}

	printer.PrintInfo("Token ID:    %s", token.Id)
	printer.PrintInfo("Token:       %s", token.Token)
	printer.PrintInfo("Description: %s", token.Description)
	printer.PrintInfo("")
	printer.PrintInfo("Save this token now. You won't be able to see it again.")
	return nil
}

func authTokenListRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	tokens, _, err := apiClient.GetUserAccessTokensForUser(ctx, currentUser.Id, 0, 200)
	if err != nil {
		return fmt.Errorf("listing tokens: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(tokens)
		return nil
	}

	var rows [][]string
	for _, token := range tokens {
		active := "active"
		if !token.IsActive {
			active = "disabled"
		}
		rows = append(rows, []string{token.Id, token.Description, active})
	}
	printer.PrintTable([]string{"ID", "DESCRIPTION", "STATUS"}, rows)
	return nil
}

func authTokenRevokeRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, err = apiClient.RevokeUserAccessToken(ctx, args[0])
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}

	printer.PrintSuccess("Revoked token %s", args[0])
	return nil
}
