package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fatih/color"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	notifyCommand := &cobra.Command{
		Use:   "notify",
		Short: "Stream real-time notifications via WebSocket",
		RunE:  notifyRun,
	}
	notifyCommand.Flags().StringArray("event", nil, "Filter to specific event types (e.g., posted, typing)")
	notifyCommand.Flags().String("channel", "", "Filter to a specific channel name")

	rootCommand.AddCommand(notifyCommand)
}

func notifyRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	websocketUrl := client.WebSocketUrl(server.URL)

	websocketClient, err := model.NewWebSocketClient4(websocketUrl, server.Token)
	if err != nil {
		return fmt.Errorf("connecting to websocket: %w", err)
	}
	defer websocketClient.Close()

	websocketClient.Listen()

	eventFilter, _ := command.Flags().GetStringArray("event")
	channelFilter, _ := command.Flags().GetString("channel")

	var channelFilterId string
	if channelFilter != "" && server.TeamID != "" {
		channelId, err := resolveChannelId(ctx, apiClient, server.TeamID, channelFilter)
		if err == nil {
			channelFilterId = channelId
		}
	}

	eventFilterSet := make(map[string]bool)
	for _, eventType := range eventFilter {
		eventFilterSet[eventType] = true
	}

	userCache := make(map[string]string)
	resolveUsername := func(userId string) string {
		if username, ok := userCache[userId]; ok {
			return username
		}
		user, _, err := apiClient.GetUser(ctx, userId, "")
		if err == nil {
			userCache[userId] = user.Username
			return user.Username
		}
		return userId[:8]
	}

	channelCache := make(map[string]string)
	resolveChannelName := func(channelId string) string {
		if channelName, ok := channelCache[channelId]; ok {
			return channelName
		}
		channel, _, err := apiClient.GetChannel(ctx, channelId)
		if err == nil {
			channelCache[channelId] = channel.DisplayName
			return channel.DisplayName
		}
		return channelId[:8]
	}

	bold := color.New(color.Bold)
	dim := color.New(color.Faint)

	printer.PrintInfo("Listening for events... (Ctrl+C to stop)")

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-signalChannel:
			printer.PrintInfo("\nDisconnected.")
			return nil

		case event, ok := <-websocketClient.EventChannel:
			if !ok {
				return fmt.Errorf("websocket connection closed")
			}

			eventType := string(event.EventType())

			if len(eventFilterSet) > 0 && !eventFilterSet[eventType] {
				continue
			}
			if channelFilterId != "" {
				broadcast := event.GetBroadcast()
				if broadcast != nil && broadcast.ChannelId != channelFilterId {
					continue
				}
			}

			if printer.JSONOutput {
				printer.PrintJSON(event)
				continue
			}

			switch eventType {
			case "posted":
				data := event.GetData()
				postJSON, _ := data["post"].(string)
				var post model.Post
				if err := json.Unmarshal([]byte(postJSON), &post); err != nil {
					continue
				}
				channelName, _ := data["channel_display_name"].(string)
				if channelName == "" {
					channelName = resolveChannelName(post.ChannelId)
				}
				senderName, _ := data["sender_name"].(string)
				if senderName == "" {
					senderName = resolveUsername(post.UserId)
				}
				senderName = strings.TrimPrefix(senderName, "@")

				prefix := ""
				if post.RootId != "" {
					prefix = dim.Sprint("[reply] ")
				}

				fmt.Fprintf(printer.Stdout, "%s %s%s %s: %s\n",
					dim.Sprint(printer.FormatTime(post.CreateAt)),
					prefix,
					bold.Sprintf("#%s", channelName),
					bold.Sprint(senderName),
					post.Message,
				)

			case "typing":
				data := event.GetData()
				userId, _ := data["user_id"].(string)
				broadcast := event.GetBroadcast()
				channelId := ""
				if broadcast != nil {
					channelId = broadcast.ChannelId
				}
				fmt.Fprintf(printer.Stdout, "%s #%s %s is typing...\n",
					dim.Sprint(printer.FormatTime(0)),
					resolveChannelName(channelId),
					resolveUsername(userId),
				)

			case "status_change":
				data := event.GetData()
				userId, _ := data["user_id"].(string)
				status, _ := data["status"].(string)
				fmt.Fprintf(printer.Stdout, "%s %s is now %s\n",
					dim.Sprint("status"),
					resolveUsername(userId),
					status,
				)

			case "reaction_added", "reaction_removed":
				data := event.GetData()
				reactionJSON, _ := data["reaction"].(string)
				var reaction model.Reaction
				if err := json.Unmarshal([]byte(reactionJSON), &reaction); err != nil {
					continue
				}
				action := "reacted"
				if eventType == "reaction_removed" {
					action = "unreacted"
				}
				fmt.Fprintf(printer.Stdout, "%s %s %s :%s: on %s\n",
					dim.Sprint("reaction"),
					resolveUsername(reaction.UserId),
					action,
					reaction.EmojiName,
					reaction.PostId[:8],
				)

			case "channel_created", "channel_deleted", "channel_updated":
				broadcast := event.GetBroadcast()
				channelId := ""
				if broadcast != nil {
					channelId = broadcast.ChannelId
				}
				fmt.Fprintf(printer.Stdout, "%s channel %s: %s\n",
					dim.Sprint("channel"),
					eventType,
					resolveChannelName(channelId),
				)

			case "user_added", "user_removed":
				data := event.GetData()
				userId, _ := data["user_id"].(string)
				broadcast := event.GetBroadcast()
				channelId := ""
				if broadcast != nil {
					channelId = broadcast.ChannelId
				}
				fmt.Fprintf(printer.Stdout, "%s %s %s in %s\n",
					dim.Sprint("member"),
					resolveUsername(userId),
					strings.TrimPrefix(eventType, "user_"),
					resolveChannelName(channelId),
				)

			default:
				fmt.Fprintf(printer.Stdout, "%s %s\n",
					dim.Sprint("event"),
					eventType,
				)
			}
		}
	}
}
