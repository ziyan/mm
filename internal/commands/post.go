package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/client"
	"github.com/ziyan/mm/internal/printer"
)

func init() {
	postCommand := &cobra.Command{
		Use:     "post",
		Aliases: []string{"msg"},
		Short:   "Manage posts/messages",
	}

	createCommand := &cobra.Command{
		Use:   "create <channel> <message>",
		Short: "Post a message to a channel",
		Args:  cobra.MinimumNArgs(1),
		RunE:  postCreateRun,
	}
	createCommand.Flags().StringArrayP("file", "f", nil, "Attach file(s)")
	createCommand.Flags().String("root-id", "", "Reply to a thread (post ID)")

	listCommand := &cobra.Command{
		Use:   "list <channel>",
		Short: "List recent messages in a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  postListRun,
	}
	listCommand.Flags().IntP("count", "n", 20, "Number of messages")

	threadCommand := &cobra.Command{
		Use:   "thread <post-id>",
		Short: "View a thread",
		Args:  cobra.ExactArgs(1),
		RunE:  postThreadRun,
	}

	replyCommand := &cobra.Command{
		Use:   "reply <post-id> <message>",
		Short: "Reply to a thread",
		Args:  cobra.MinimumNArgs(2),
		RunE:  postReplyRun,
	}

	editCommand := &cobra.Command{
		Use:   "edit <post-id> <new-message>",
		Short: "Edit a post",
		Args:  cobra.MinimumNArgs(2),
		RunE:  postEditRun,
	}

	deleteCommand := &cobra.Command{
		Use:   "delete <post-id>",
		Short: "Delete a post",
		Args:  cobra.ExactArgs(1),
		RunE:  postDeleteRun,
	}

	pinCommand := &cobra.Command{
		Use:   "pin <post-id>",
		Short: "Pin a post",
		Args:  cobra.ExactArgs(1),
		RunE:  postPinRun,
	}

	unpinCommand := &cobra.Command{
		Use:   "unpin <post-id>",
		Short: "Unpin a post",
		Args:  cobra.ExactArgs(1),
		RunE:  postUnpinRun,
	}

	reactCommand := &cobra.Command{
		Use:   "react <post-id> <emoji-name>",
		Short: "Add a reaction to a post",
		Args:  cobra.ExactArgs(2),
		RunE:  postReactRun,
	}

	unreactCommand := &cobra.Command{
		Use:   "unreact <post-id> <emoji-name>",
		Short: "Remove a reaction from a post",
		Args:  cobra.ExactArgs(2),
		RunE:  postUnreactRun,
	}

	searchCommand := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for posts",
		Args:  cobra.MinimumNArgs(1),
		RunE:  postSearchRun,
	}
	searchCommand.Flags().Bool("or", false, "Use OR instead of AND for terms")

	pinnedCommand := &cobra.Command{
		Use:   "pinned <channel>",
		Short: "List pinned posts in a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  postPinnedRun,
	}

	unreadCommand := &cobra.Command{
		Use:   "unread <channel>",
		Short: "List unread messages in a channel",
		Args:  cobra.ExactArgs(1),
		RunE:  postUnreadRun,
	}
	unreadCommand.Flags().IntP("before", "b", 0, "Include N messages before the unread boundary")

	postCommand.AddCommand(createCommand, listCommand, threadCommand, replyCommand, editCommand, deleteCommand, pinCommand, unpinCommand, reactCommand, unreactCommand, searchCommand, pinnedCommand, unreadCommand)
	rootCommand.AddCommand(postCommand)
}

func formatPost(apiClient *model.Client4, ctx context.Context, post *model.Post, userCache map[string]string) string {
	username := post.UserId
	if userCache != nil {
		if name, ok := userCache[post.UserId]; ok {
			username = name
		} else if user, _, err := apiClient.GetUser(ctx, post.UserId, ""); err == nil {
			username = user.Username
			userCache[post.UserId] = username
		}
	}

	timestamp := printer.FormatTime(post.CreateAt)
	message := post.Message

	if len(post.FileIds) > 0 {
		message += fmt.Sprintf(" [%d file(s)]", len(post.FileIds))
	}

	if post.Metadata != nil && len(post.Metadata.Reactions) > 0 {
		reactionCounts := make(map[string]int)
		for _, reaction := range post.Metadata.Reactions {
			reactionCounts[reaction.EmojiName]++
		}
		var parts []string
		for emojiName, count := range reactionCounts {
			parts = append(parts, fmt.Sprintf(":%s: %d", emojiName, count))
		}
		message += " " + strings.Join(parts, " ")
	}

	prefix := ""
	if post.RootId != "" {
		prefix = "  ↳ "
	}

	return fmt.Sprintf("%s%s  %s  %s  %s", prefix, post.Id[:8], timestamp, username, message)
}

func postCreateRun(command *cobra.Command, args []string) error {
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

	var message string
	if len(args) > 1 {
		message = strings.Join(args[1:], " ")
	} else {
		data, err := os.ReadFile("/dev/stdin")
		if err != nil {
			return fmt.Errorf("no message provided and cannot read stdin")
		}
		message = string(data)
	}

	rootId, _ := command.Flags().GetString("root-id")
	filePaths, _ := command.Flags().GetStringArray("file")

	post := &model.Post{
		ChannelId: channelId,
		Message:   message,
		RootId:    rootId,
	}

	for _, filePath := range filePaths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", filePath, err)
		}
		response, _, err := apiClient.UploadFile(ctx, data, channelId, filePath)
		if err != nil {
			return fmt.Errorf("uploading %s: %w", filePath, err)
		}
		post.FileIds = append(post.FileIds, response.FileInfos[0].Id)
	}

	created, _, err := apiClient.CreatePost(ctx, post)
	if err != nil {
		return fmt.Errorf("creating post: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(created)
		return nil
	}

	printer.PrintSuccess("Posted %s to %s", created.Id[:8], args[0])
	return nil
}

func postListRun(command *cobra.Command, args []string) error {
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

	count, _ := command.Flags().GetInt("count")
	postList, _, err := apiClient.GetPostsForChannel(ctx, channelId, 0, count, "", false, false)
	if err != nil {
		return fmt.Errorf("listing posts: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postList)
		return nil
	}

	userCache := make(map[string]string)
	for index := len(postList.Order) - 1; index >= 0; index-- {
		post := postList.Posts[postList.Order[index]]
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

func postThreadRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	postList, _, err := apiClient.GetPostThread(ctx, postId, "", false)
	if err != nil {
		return fmt.Errorf("getting thread: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postList)
		return nil
	}

	userCache := make(map[string]string)
	for index := len(postList.Order) - 1; index >= 0; index-- {
		post := postList.Posts[postList.Order[index]]
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

func postReplyRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	rootPost, _, err := apiClient.GetPost(ctx, postId, "")
	if err != nil {
		return fmt.Errorf("post not found: %w", err)
	}

	rootId := rootPost.Id
	if rootPost.RootId != "" {
		rootId = rootPost.RootId
	}

	message := strings.Join(args[1:], " ")
	created, _, err := apiClient.CreatePost(ctx, &model.Post{
		ChannelId: rootPost.ChannelId,
		Message:   message,
		RootId:    rootId,
	})
	if err != nil {
		return fmt.Errorf("replying: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(created)
		return nil
	}

	printer.PrintSuccess("Replied %s in thread %s", created.Id[:8], rootId[:8])
	return nil
}

func postEditRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])
	message := strings.Join(args[1:], " ")

	patch := &model.PostPatch{
		Message: &message,
	}

	updated, _, err := apiClient.PatchPost(ctx, postId, patch)
	if err != nil {
		return fmt.Errorf("editing post: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(updated)
		return nil
	}

	printer.PrintSuccess("Edited post %s", updated.Id[:8])
	return nil
}

func postDeleteRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	_, err = apiClient.DeletePost(ctx, postId)
	if err != nil {
		return fmt.Errorf("deleting post: %w", err)
	}

	printer.PrintSuccess("Deleted post %s", args[0])
	return nil
}

func postPinRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	_, err = apiClient.PinPost(ctx, postId)
	if err != nil {
		return fmt.Errorf("pinning post: %w", err)
	}

	printer.PrintSuccess("Pinned post %s", args[0])
	return nil
}

func postUnpinRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	_, err = apiClient.UnpinPost(ctx, postId)
	if err != nil {
		return fmt.Errorf("unpinning post: %w", err)
	}

	printer.PrintSuccess("Unpinned post %s", args[0])
	return nil
}

func postReactRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	_, _, err = apiClient.SaveReaction(ctx, &model.Reaction{
		UserId:    currentUser.Id,
		PostId:    postId,
		EmojiName: strings.Trim(args[1], ":"),
	})
	if err != nil {
		return fmt.Errorf("adding reaction: %w", err)
	}

	printer.PrintSuccess("Reacted :%s: to %s", args[1], args[0])
	return nil
}

func postUnreactRun(command *cobra.Command, args []string) error {
	apiClient, _, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	postId := normalizePostId(args[0])

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	_, err = apiClient.DeleteReaction(ctx, &model.Reaction{
		UserId:    currentUser.Id,
		PostId:    postId,
		EmojiName: strings.Trim(args[1], ":"),
	})
	if err != nil {
		return fmt.Errorf("removing reaction: %w", err)
	}

	printer.PrintSuccess("Removed :%s: from %s", args[1], args[0])
	return nil
}

func postSearchRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	query := strings.Join(args, " ")
	isOrSearch, _ := command.Flags().GetBool("or")

	postList, _, err := apiClient.SearchPosts(ctx, teamId, query, isOrSearch)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postList)
		return nil
	}

	if len(postList.Order) == 0 {
		printer.PrintInfo("No results found")
		return nil
	}

	userCache := make(map[string]string)
	for _, postId := range postList.Order {
		post := postList.Posts[postId]
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

func postPinnedRun(command *cobra.Command, args []string) error {
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

	postList, _, err := apiClient.GetPinnedPosts(ctx, channelId, "")
	if err != nil {
		return fmt.Errorf("getting pinned posts: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postList)
		return nil
	}

	if len(postList.Order) == 0 {
		printer.PrintInfo("No pinned posts")
		return nil
	}

	userCache := make(map[string]string)
	for _, postId := range postList.Order {
		post := postList.Posts[postId]
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

func postUnreadRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])
	if err != nil {
		return err
	}

	contextBefore, _ := command.Flags().GetInt("before")

	postList, _, err := apiClient.GetPostsAroundLastUnread(ctx, currentUser.Id, channelId, contextBefore, 200, false)
	if err != nil {
		return fmt.Errorf("getting unread posts: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(postList)
		return nil
	}

	if len(postList.Order) == 0 {
		printer.PrintInfo("No unread messages in %s", args[0])
		return nil
	}

	userCache := make(map[string]string)
	for index := len(postList.Order) - 1; index >= 0; index-- {
		post := postList.Posts[postList.Order[index]]
		_, _ = fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
	}
	return nil
}

// normalizePostId returns the post ID as-is (full 26-char Mattermost IDs)
func normalizePostId(id string) string {
	return id
}

// PostFromJSON unmarshals a post from a JSON string (used by websocket events)
func PostFromJSON(data string) (*model.Post, error) {
	var post model.Post
	if err := json.Unmarshal([]byte(data), &post); err != nil {
		return nil, err
	}
	return &post, nil
}
