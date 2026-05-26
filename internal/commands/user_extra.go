package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	avatarCommand := &cobra.Command{
		Use:   "avatar",
		Short: "Profile image operations",
	}

	avatarGetCommand := &cobra.Command{
		Use:   "get [username] [output-path]",
		Short: "Download profile image (default: your own)",
		Args:  cobra.MaximumNArgs(2),
		RunE:  userAvatarGetRun,
	}

	avatarSetCommand := &cobra.Command{
		Use:   "set <image-file>",
		Short: "Set your profile image",
		Args:  cobra.ExactArgs(1),
		RunE:  userAvatarSetRun,
	}

	avatarResetCommand := &cobra.Command{
		Use:   "reset",
		Short: "Reset to default profile image",
		RunE:  userAvatarResetRun,
	}

	avatarCommand.AddCommand(avatarGetCommand, avatarSetCommand, avatarResetCommand)

	typingCommand := &cobra.Command{
		Use:   "typing <channel>",
		Short: "Send a typing indicator to a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  userTypingRun,
	}

	autocompleteCommand := &cobra.Command{
		Use:   "autocomplete <prefix>",
		Short: "Autocomplete usernames",
		Args:  cobra.ExactArgs(1),
		RunE:  userAutocompleteRun,
	}
	autocompleteCommand.Flags().String("channel", "", "Limit to a channel")

	for _, child := range rootCommand.Commands() {
		if child.Name() == "user" {
			child.AddCommand(avatarCommand, typingCommand, autocompleteCommand)
			break
		}
	}
}

func userAvatarGetRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	var userId string
	if len(arguments) > 0 {
		user, _, err := apiClient.GetUserByUsername(ctx, arguments[0], "")
		if err != nil {
			return fmt.Errorf("commands: user not found: %w", err)
		}
		userId = user.Id
	} else {
		currentUser, _, err := apiClient.GetMe(ctx, "")
		if err != nil {
			return err
		}
		userId = currentUser.Id
	}

	data, _, err := apiClient.GetProfileImage(ctx, userId, "")
	if err != nil {
		return fmt.Errorf("commands: getting profile image: %w", err)
	}

	outputPath := "avatar.png"
	if len(arguments) > 1 {
		outputPath = arguments[1]
	} else if len(arguments) > 0 {
		outputPath = arguments[0] + ".png"
	}

	if outputPath == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("commands: writing file: %w", err)
	}

	printer.PrintSuccess("Downloaded profile image to %s (%d bytes)", outputPath, len(data))
	return nil
}

func userAvatarSetRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	imageData, err := os.ReadFile(arguments[0])
	if err != nil {
		return fmt.Errorf("commands: reading image: %w", err)
	}

	_, err = apiClient.SetProfileImage(ctx, currentUser.Id, imageData)
	if err != nil {
		return fmt.Errorf("commands: setting profile image: %w", err)
	}

	printer.PrintSuccess("Profile image updated")
	return nil
}

func userAvatarResetRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	_, err = apiClient.SetDefaultProfileImage(ctx, currentUser.Id)
	if err != nil {
		return fmt.Errorf("commands: resetting profile image: %w", err)
	}

	printer.PrintSuccess("Profile image reset to default")
	return nil
}

func userTypingRun(command *cobra.Command, arguments []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, arguments[0])
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	_, err = apiClient.PublishUserTyping(ctx, currentUser.Id, model.TypingRequest{
		ChannelId: channelId,
	})
	if err != nil {
		return fmt.Errorf("commands: sending typing indicator: %w", err)
	}

	printer.PrintSuccess("Sent typing indicator to %s", arguments[0])
	return nil
}

func userAutocompleteRun(command *cobra.Command, arguments []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	channelFilter, _ := command.Flags().GetString("channel")

	var result *model.UserAutocomplete
	if channelFilter != "" {
		teamId, err := resolveTeamId(ctx, command, apiClient, server)
		if err != nil {
			return err
		}
		channelId, err := resolveChannelId(ctx, apiClient, teamId, channelFilter)
		if err != nil {
			return err
		}
		result, _, err = apiClient.AutocompleteUsersInChannel(ctx, server.TeamID, channelId, arguments[0], 25, "")
		if err != nil {
			return fmt.Errorf("commands: autocompleting: %w", err)
		}
	} else if server.TeamID != "" {
		var err error
		result, _, err = apiClient.AutocompleteUsersInTeam(ctx, server.TeamID, arguments[0], 25, "")
		if err != nil {
			return fmt.Errorf("commands: autocompleting: %w", err)
		}
	} else {
		var err error
		result, _, err = apiClient.AutocompleteUsers(ctx, arguments[0], 25, "")
		if err != nil {
			return fmt.Errorf("commands: autocompleting: %w", err)
		}
	}

	if printer.JSONOutput {
		printer.PrintJSON(result)
		return nil
	}

	var rows [][]string
	if result.Users != nil {
		for _, user := range result.Users {
			rows = append(rows, []string{user.Username, user.GetFullName(), "member"})
		}
	}
	if result.OutOfChannel != nil {
		for _, user := range result.OutOfChannel {
			rows = append(rows, []string{user.Username, user.GetFullName(), "not in channel"})
		}
	}
	printer.PrintTable([]string{"USERNAME", "NAME", "STATUS"}, rows)
	return nil
}
