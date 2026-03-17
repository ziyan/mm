package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/config"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	authCommand := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	loginCommand := &cobra.Command{
		Use:   "login",
		Short: "Login to a Mattermost server",
		RunE:  authLoginRun,
	}
	loginCommand.Flags().String("url", "", "Server URL (required)")
	loginCommand.Flags().String("name", "", "Profile name (default: hostname)")
	loginCommand.Flags().StringP("token", "t", "", "Personal access token")
	loginCommand.Flags().StringP("user", "u", "", "Username or email (for password login)")
	loginCommand.Flags().StringP("password", "p", "", "Password (for password login)")
	_ = loginCommand.MarkFlagRequired("url")

	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status",
		RunE:  authStatusRun,
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		RunE:  authListRun,
	}

	switchCommand := &cobra.Command{
		Use:   "switch <profile>",
		Short: "Switch active profile",
		Args:  cobra.ExactArgs(1),
		RunE:  authSwitchRun,
	}

	removeCommand := &cobra.Command{
		Use:   "remove <profile>",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE:  authRemoveRun,
	}

	authCommand.AddCommand(loginCommand, statusCommand, listCommand, switchCommand, removeCommand)
	rootCommand.AddCommand(authCommand)
}

func authLoginRun(command *cobra.Command, args []string) error {
	serverURL, _ := command.Flags().GetString("url")
	profileName, _ := command.Flags().GetString("name")
	token, _ := command.Flags().GetString("token")
	username, _ := command.Flags().GetString("user")
	password, _ := command.Flags().GetString("password")

	serverURL = strings.TrimRight(serverURL, "/")
	if !strings.HasPrefix(serverURL, "http") {
		serverURL = "https://" + serverURL
	}

	if profileName == "" {
		profileName = strings.TrimPrefix(serverURL, "https://")
		profileName = strings.TrimPrefix(profileName, "http://")
		profileName = strings.Split(profileName, "/")[0]
		profileName = strings.Split(profileName, ":")[0]
	}

	apiClient := model.NewAPIv4Client(serverURL)
	ctx := context.Background()

	if token != "" {
		apiClient.SetToken(token)
	} else if username != "" && password != "" {
		user, _, err := apiClient.Login(ctx, username, password)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
		token = apiClient.AuthToken
		printer.PrintInfo("Logged in as %s (%s)", user.Username, user.Email)
	} else {
		return fmt.Errorf("provide --token or --user and --password")
	}

	// Verify token
	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}

	configuration, err := config.Load()
	if err != nil {
		return err
	}

	configuration.SetProfile(profileName, config.ServerProfile{
		URL:      serverURL,
		Token:    token,
		Username: currentUser.Username,
	})
	configuration.ActiveProfile = profileName

	if err := configuration.Save(); err != nil {
		return err
	}

	printer.PrintSuccess("Logged in to %s as %s (profile: %s)", serverURL, currentUser.Username, profileName)
	return nil
}

func authStatusRun(command *cobra.Command, args []string) error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	server, err := configuration.ActiveServer()
	if err != nil {
		return err
	}

	if printer.JSONOutput {
		printer.PrintJSON(server)
		return nil
	}

	printer.PrintInfo("Profile:  %s", server.Name)
	printer.PrintInfo("Server:   %s", server.URL)
	printer.PrintInfo("Username: %s", server.Username)
	if server.TeamName != "" {
		printer.PrintInfo("Team:     %s", server.TeamName)
	}
	return nil
}

func authListRun(command *cobra.Command, args []string) error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}

	if printer.JSONOutput {
		printer.PrintJSON(configuration.Profiles)
		return nil
	}

	if len(configuration.Profiles) == 0 {
		printer.PrintInfo("No profiles configured. Run: mm auth login")
		return nil
	}

	var rows [][]string
	for _, profile := range configuration.Profiles {
		active := ""
		if profile.Name == configuration.ActiveProfile {
			active = "*"
		}
		rows = append(rows, []string{active, profile.Name, profile.URL, profile.Username})
	}
	printer.PrintTable([]string{"", "NAME", "URL", "USER"}, rows)
	return nil
}

func authSwitchRun(command *cobra.Command, args []string) error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	profileName := args[0]
	if _, ok := configuration.Profiles[profileName]; !ok {
		return fmt.Errorf("profile %q not found", profileName)
	}
	configuration.ActiveProfile = profileName
	if err := configuration.Save(); err != nil {
		return err
	}
	printer.PrintSuccess("Switched to profile %s", profileName)
	return nil
}

func authRemoveRun(command *cobra.Command, args []string) error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	profileName := args[0]
	if _, ok := configuration.Profiles[profileName]; !ok {
		return fmt.Errorf("profile %q not found", profileName)
	}
	delete(configuration.Profiles, profileName)
	if configuration.ActiveProfile == profileName {
		configuration.ActiveProfile = ""
		for key := range configuration.Profiles {
			configuration.ActiveProfile = key
			break
		}
	}
	if err := configuration.Save(); err != nil {
		return err
	}
	printer.PrintSuccess("Removed profile %s", profileName)
	return nil
}
