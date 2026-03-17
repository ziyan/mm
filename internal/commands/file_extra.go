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
	searchCommand := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for files",
		Args:  cobra.MinimumNArgs(1),
		RunE:  fileSearchRun,
	}

	for _, child := range rootCommand.Commands() {
		if child.Name() == "file" {
			child.AddCommand(searchCommand)
			break
		}
	}
}

func fileSearchRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	query := args[0]

	results, _, err := apiClient.SearchFiles(ctx, teamId, query, false)
	if err != nil {
		return fmt.Errorf("searching files: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(results)
		return nil
	}

	if len(results.Order) == 0 {
		printer.PrintInfo("No files found")
		return nil
	}

	var rows [][]string
	for _, fileId := range results.Order {
		fileInfo := results.FileInfos[fileId]
		rows = append(rows, formatFileInfoRow(fileInfo))
	}
	printer.PrintTable([]string{"NAME", "SIZE", "TYPE", "ID"}, rows)
	return nil
}

func formatFileInfoRow(fileInfo *model.FileInfo) []string {
	size := fmt.Sprintf("%d B", fileInfo.Size)
	if fileInfo.Size > 1024*1024 {
		size = fmt.Sprintf("%.1f MB", float64(fileInfo.Size)/(1024*1024))
	} else if fileInfo.Size > 1024 {
		size = fmt.Sprintf("%.1f KB", float64(fileInfo.Size)/1024)
	}
	return []string{fileInfo.Name, size, fileInfo.MimeType, fileInfo.Id}
}
