package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	emojiCommand := &cobra.Command{
		Use:   "emoji",
		Short: "Custom emoji operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List custom emoji",
		RunE:  emojiListRun,
	}
	listCommand.Flags().IntP("count", "n", 50, "Number of emoji")

	createCommand := &cobra.Command{
		Use:   "create <name> <image-file>",
		Short: "Create a custom emoji",
		Args:  cobra.ExactArgs(2),
		RunE:  emojiCreateRun,
	}

	deleteCommand := &cobra.Command{
		Use:   "delete <emoji-name>",
		Short: "Delete a custom emoji",
		Args:  cobra.ExactArgs(1),
		RunE:  emojiDeleteRun,
	}

	searchCommand := &cobra.Command{
		Use:   "search <query>",
		Short: "Search emoji by name",
		Args:  cobra.ExactArgs(1),
		RunE:  emojiSearchRun,
	}

	emojiCommand.AddCommand(listCommand, createCommand, deleteCommand, searchCommand)
	rootCommand.AddCommand(emojiCommand)
}

func emojiListRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	count, _ := command.Flags().GetInt("count")
	emojis, _, err := apiClient.GetEmojiList(ctx, 0, count)
	if err != nil {
		return fmt.Errorf("commands: listing emoji: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(emojis)
		return nil
	}

	var rows [][]string
	for _, emoji := range emojis {
		rows = append(rows, []string{emoji.Name, emoji.CreatorId, emoji.Id})
	}
	printer.PrintTable([]string{"NAME", "CREATOR", "ID"}, rows)
	return nil
}

func emojiCreateRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	imageData, err := os.ReadFile(arguments[1])
	if err != nil {
		return fmt.Errorf("commands: reading image: %w", err)
	}

	emoji, _, err := apiClient.CreateEmoji(ctx, &model.Emoji{
		Name:      arguments[0],
		CreatorId: currentUser.Id,
	}, imageData, filepath.Base(arguments[1]))
	if err != nil {
		return fmt.Errorf("commands: creating emoji: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(emoji)
		return nil
	}

	printer.PrintSuccess("Created emoji :%s: (%s)", emoji.Name, emoji.Id)
	return nil
}

func emojiDeleteRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	emoji, _, err := apiClient.GetEmojiByName(ctx, arguments[0])
	if err != nil {
		return fmt.Errorf("commands: emoji not found: %w", err)
	}

	_, err = apiClient.DeleteEmoji(ctx, emoji.Id)
	if err != nil {
		return fmt.Errorf("commands: deleting emoji: %w", err)
	}

	printer.PrintSuccess("Deleted emoji :%s:", arguments[0])
	return nil
}

func emojiSearchRun(command *cobra.Command, arguments []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	emojis, _, err := apiClient.AutocompleteEmoji(ctx, arguments[0], "")
	if err != nil {
		return fmt.Errorf("commands: searching emoji: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(emojis)
		return nil
	}

	var rows [][]string
	for _, emoji := range emojis {
		rows = append(rows, []string{emoji.Name, emoji.CreatorId, emoji.Id})
	}
	printer.PrintTable([]string{"NAME", "CREATOR", "ID"}, rows)
	return nil
}
