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
	fileCommand := &cobra.Command{
		Use:   "file",
		Short: "File operations",
	}

	uploadCommand := &cobra.Command{
		Use:   "upload <channel> <file-path>...",
		Short: "Upload file(s) to a channel",
		Args:  cobra.MinimumNArgs(2),
		RunE:  fileUploadRun,
	}
	uploadCommand.Flags().StringP("message", "m", "", "Message to accompany the file")

	downloadCommand := &cobra.Command{
		Use:   "download <file-id> [output-path]",
		Short: "Download a file",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  fileDownloadRun,
	}

	infoCommand := &cobra.Command{
		Use:   "info <file-id>",
		Short: "Show file info",
		Args:  cobra.ExactArgs(1),
		RunE:  fileInfoRun,
	}

	fileCommand.AddCommand(uploadCommand, downloadCommand, infoCommand)
	rootCommand.AddCommand(fileCommand)
}

func fileUploadRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	message, _ := command.Flags().GetString("message")

	var fileIds []string
	for _, filePath := range args[1:] {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filePath, err)
		}
		response, _, err := apiClient.UploadFile(ctx, data, channelId, filepath.Base(filePath))
		if err != nil {
			return fmt.Errorf("uploading %s: %w", filePath, err)
		}
		fileIds = append(fileIds, response.FileInfos[0].Id)
		printer.PrintInfo("Uploaded %s (%s)", filepath.Base(filePath), response.FileInfos[0].Id)
	}

	post := &model.Post{
		ChannelId: channelId,
		Message:   message,
		FileIds:   fileIds,
	}
	_, _, err = apiClient.CreatePost(ctx, post)
	if err != nil {
		return fmt.Errorf("creating post: %w", err)
	}

	printer.PrintSuccess("Uploaded %d file(s) to %s", len(fileIds), args[0])
	return nil
}

func fileDownloadRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	fileId := args[0]

	fileInfo, _, err := apiClient.GetFileInfo(ctx, fileId)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	outputPath := fileInfo.Name
	if len(args) > 1 {
		outputPath = args[1]
	}

	data, _, err := apiClient.GetFile(ctx, fileId)
	if err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}

	if outputPath == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	printer.PrintSuccess("Downloaded %s (%d bytes)", outputPath, len(data))
	return nil
}

func fileInfoRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	fileInfo, _, err := apiClient.GetFileInfo(ctx, args[0])
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(fileInfo)
		return nil
	}

	printer.PrintInfo("Name:      %s", fileInfo.Name)
	printer.PrintInfo("Size:      %d bytes", fileInfo.Size)
	printer.PrintInfo("Extension: %s", fileInfo.Extension)
	printer.PrintInfo("MimeType:  %s", fileInfo.MimeType)
	printer.PrintInfo("ID:        %s", fileInfo.Id)
	printer.PrintInfo("Post ID:   %s", fileInfo.PostId)
	printer.PrintInfo("Created:   %s", printer.FormatTime(fileInfo.CreateAt))
	return nil
}
