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
	preferenceCommand := &cobra.Command{
		Use:   "preference",
		Short: "User preference operations",
	}

	listCommand := &cobra.Command{
		Use:   "list [category]",
		Short: "List preferences, optionally filtered by category",
		Args:  cobra.MaximumNArgs(1),
		RunE:  preferenceListRun,
	}

	setCommand := &cobra.Command{
		Use:   "set <category> <name> <value>",
		Short: "Set a preference",
		Args:  cobra.ExactArgs(3),
		RunE:  preferenceSetRun,
	}

	deleteCommand := &cobra.Command{
		Use:   "delete <category> <name>",
		Short: "Delete a preference",
		Args:  cobra.ExactArgs(2),
		RunE:  preferenceDeleteRun,
	}

	preferenceCommand.AddCommand(listCommand, setCommand, deleteCommand)
	rootCommand.AddCommand(preferenceCommand)
}

func preferenceListRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	var preferences model.Preferences
	if len(args) > 0 {
		preferences, _, err = apiClient.GetPreferencesByCategory(ctx, currentUser.Id, args[0])
	} else {
		preferences, _, err = apiClient.GetPreferences(ctx, currentUser.Id)
	}
	if err != nil {
		return fmt.Errorf("listing preferences: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(preferences)
		return nil
	}

	if len(preferences) == 0 {
		printer.PrintInfo("No preferences")
		return nil
	}

	var rows [][]string
	for _, preference := range preferences {
		rows = append(rows, []string{preference.Category, preference.Name, printer.Truncate(preference.Value, 60)})
	}
	printer.PrintTable([]string{"CATEGORY", "NAME", "VALUE"}, rows)
	return nil
}

func preferenceSetRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	preferences := model.Preferences{
		{
			UserId:   currentUser.Id,
			Category: args[0],
			Name:     args[1],
			Value:    args[2],
		},
	}

	_, err = apiClient.UpdatePreferences(ctx, currentUser.Id, preferences)
	if err != nil {
		return fmt.Errorf("setting preference: %w", err)
	}

	printer.PrintSuccess("Set preference %s/%s", args[0], args[1])
	return nil
}

func preferenceDeleteRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	preferences := model.Preferences{
		{
			UserId:   currentUser.Id,
			Category: args[0],
			Name:     args[1],
		},
	}

	_, err = apiClient.DeletePreferences(ctx, currentUser.Id, preferences)
	if err != nil {
		return fmt.Errorf("deleting preference: %w", err)
	}

	printer.PrintSuccess("Deleted preference %s/%s", args[0], args[1])
	return nil
}
