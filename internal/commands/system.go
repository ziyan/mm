package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	serverCommand := &cobra.Command{
		Use:   "server",
		Short: "Server information and admin operations",
	}

	pingCommand := &cobra.Command{
		Use:   "ping",
		Short: "Check server connectivity",
		RunE:  serverPingRun,
	}

	infoCommand := &cobra.Command{
		Use:   "info",
		Short: "Show server information",
		RunE:  serverInfoRun,
	}

	serverCommand.AddCommand(pingCommand, infoCommand)
	rootCommand.AddCommand(serverCommand)
}

func serverPingRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	status, _, err := apiClient.GetPingWithFullServerStatus(ctx)
	if err != nil {
		return fmt.Errorf("server unreachable: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(status)
		return nil
	}

	printer.PrintSuccess("Server %s is reachable", server.URL)
	for key, value := range status {
		printer.PrintInfo("  %s: %v", key, value)
	}
	return nil
}

func serverInfoRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	response, err := apiClient.DoAPIGet(ctx, "/config/client?format=old", "")
	if err != nil {
		return fmt.Errorf("getting server info: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	var clientConfig map[string]string
	if err := json.NewDecoder(response.Body).Decode(&clientConfig); err != nil {
		return fmt.Errorf("parsing server info: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(clientConfig)
		return nil
	}

	printer.PrintInfo("Server:           %s", server.URL)
	if version, ok := clientConfig["Version"]; ok {
		printer.PrintInfo("Version:          %s", version)
	}
	if buildNumber, ok := clientConfig["BuildNumber"]; ok {
		printer.PrintInfo("Build:            %s", buildNumber)
	}
	if buildDate, ok := clientConfig["BuildDate"]; ok {
		printer.PrintInfo("Build Date:       %s", buildDate)
	}
	if buildHash, ok := clientConfig["BuildHash"]; ok {
		printer.PrintInfo("Build Hash:       %s", buildHash)
	}
	if siteName, ok := clientConfig["SiteName"]; ok {
		printer.PrintInfo("Site Name:        %s", siteName)
	}
	if driverName, ok := clientConfig["SQLDriverName"]; ok {
		printer.PrintInfo("Database:         %s", driverName)
	}
	if schemaVersion, ok := clientConfig["SchemaVersion"]; ok {
		printer.PrintInfo("Schema Version:   %s", schemaVersion)
	}
	return nil
}
