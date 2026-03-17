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
	threadCommand := &cobra.Command{
		Use:   "thread",
		Short: "Thread operations",
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List your threads",
		RunE:  threadListRun,
	}
	listCommand.Flags().Bool("unread", false, "Only show unread threads")
	listCommand.Flags().IntP("count", "n", 20, "Number of threads")

	viewCommand := &cobra.Command{
		Use:   "view <thread-id>",
		Short: "View a thread",
		Args:  cobra.ExactArgs(1),
		RunE:  threadViewRun,
	}

	followCommand := &cobra.Command{
		Use:   "follow <thread-id>",
		Short: "Follow a thread",
		Args:  cobra.ExactArgs(1),
		RunE:  threadFollowRun,
	}

	unfollowCommand := &cobra.Command{
		Use:   "unfollow <thread-id>",
		Short: "Unfollow a thread",
		Args:  cobra.ExactArgs(1),
		RunE:  threadUnfollowRun,
	}

	readCommand := &cobra.Command{
		Use:   "read <thread-id>",
		Short: "Mark a thread as read",
		Args:  cobra.ExactArgs(1),
		RunE:  threadReadRun,
	}

	unreadCommand := &cobra.Command{
		Use:   "unread <post-id>",
		Short: "Mark a thread as unread from a specific post",
		Args:  cobra.ExactArgs(1),
		RunE:  threadUnreadRun,
	}

	readAllCommand := &cobra.Command{
		Use:   "read-all",
		Short: "Mark all threads as read",
		RunE:  threadReadAllRun,
	}

	threadCommand.AddCommand(listCommand, viewCommand, followCommand, unfollowCommand, readCommand, unreadCommand, readAllCommand)
	rootCommand.AddCommand(threadCommand)
}

func threadListRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	count, _ := command.Flags().GetInt("count")
	unreadOnly, _ := command.Flags().GetBool("unread")

	options := model.GetUserThreadsOpts{
		PageSize:  uint64(count),
		Unread:    unreadOnly,
		Extended:  true,
		TotalsOnly: false,
	}

	threads, _, err := apiClient.GetUserThreads(ctx, currentUser.Id, teamId, options)
	if err != nil {
		return fmt.Errorf("listing threads: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(threads)
		return nil
	}

	if len(threads.Threads) == 0 {
		printer.PrintInfo("No threads")
		return nil
	}

	var rows [][]string
	for _, thread := range threads.Threads {
		unreadMark := ""
		if thread.UnreadMentions > 0 {
			unreadMark = fmt.Sprintf("@%d", thread.UnreadMentions)
		} else if thread.UnreadReplies > 0 {
			unreadMark = fmt.Sprintf("%d new", thread.UnreadReplies)
		}

		message := ""
		if thread.Post != nil {
			message = printer.Truncate(thread.Post.Message, 60)
		}

		rows = append(rows, []string{
			thread.PostId[:8],
			fmt.Sprintf("%d", thread.ReplyCount),
			unreadMark,
			printer.FormatTime(thread.LastReplyAt),
			message,
		})
	}
	printer.PrintTable([]string{"ID", "REPLIES", "UNREAD", "LAST REPLY", "MESSAGE"}, rows)
	return nil
}

func threadViewRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	threadId := args[0]

	thread, _, err := apiClient.GetUserThread(ctx, currentUser.Id, teamId, threadId, true)
	if err != nil {
		return fmt.Errorf("getting thread: %w", err)
	}

	if printer.JSONOutput {
		printer.PrintJSON(thread)
		return nil
	}

	if thread.Post != nil {
		userCache := make(map[string]string)
		fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, thread.Post, userCache))

		postList, _, err := apiClient.GetPostThread(ctx, threadId, "", false)
		if err == nil {
			for index := len(postList.Order) - 1; index >= 0; index-- {
				post := postList.Posts[postList.Order[index]]
				if post.Id != threadId {
					fmt.Fprintln(printer.Stdout, formatPost(apiClient, ctx, post, userCache))
				}
			}
		}
	}
	return nil
}

func threadFollowRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	_, err = apiClient.UpdateThreadFollowForUser(ctx, currentUser.Id, teamId, args[0], true)
	if err != nil {
		return fmt.Errorf("following thread: %w", err)
	}

	printer.PrintSuccess("Following thread %s", args[0])
	return nil
}

func threadUnfollowRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	_, err = apiClient.UpdateThreadFollowForUser(ctx, currentUser.Id, teamId, args[0], false)
	if err != nil {
		return fmt.Errorf("unfollowing thread: %w", err)
	}

	printer.PrintSuccess("Unfollowed thread %s", args[0])
	return nil
}

func threadReadRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	_, _, err = apiClient.UpdateThreadReadForUser(ctx, currentUser.Id, teamId, args[0], model.GetMillis())
	if err != nil {
		return fmt.Errorf("marking thread as read: %w", err)
	}

	printer.PrintSuccess("Marked thread %s as read", args[0])
	return nil
}

func threadUnreadRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	_, _, err = apiClient.SetThreadUnreadByPostId(ctx, currentUser.Id, teamId, args[0], args[0])
	if err != nil {
		return fmt.Errorf("marking thread as unread: %w", err)
	}

	printer.PrintSuccess("Marked thread as unread from post %s", args[0])
	return nil
}

func threadReadAllRun(command *cobra.Command, args []string) error {
	apiClient, server, err := client.New()
	if err != nil {
		return err
	}
	ctx := context.Background()

	currentUser, _, err := apiClient.GetMe(ctx, "")
	if err != nil {
		return err
	}

	teamId, err := resolveTeamId(ctx, command, apiClient, server)
	if err != nil {
		return err
	}

	_, err = apiClient.UpdateThreadsReadForUser(ctx, currentUser.Id, teamId)
	if err != nil {
		return fmt.Errorf("marking all threads as read: %w", err)
	}

	printer.PrintSuccess("Marked all threads as read")
	return nil
}
