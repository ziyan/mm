package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/spf13/cobra"
	"github.com/ziyan/mm/internal/config"
	"github.com/ziyan/mm/internal/printer"
)

type userSummary struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	Position  string `json:"position,omitempty"`
}

func collectUserIdsFromPostList(postList *model.PostList) []string {
	seen := make(map[string]struct{})
	for _, post := range postList.Posts {
		if post.UserId != "" {
			seen[post.UserId] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

func collectUserIdsFromPosts(posts []*model.Post) []string {
	seen := make(map[string]struct{})
	for _, post := range posts {
		if post.UserId != "" {
			seen[post.UserId] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

func resolveUsersByIds(ctx context.Context, apiClient *model.Client4, userIds []string) (map[string]*userSummary, error) {
	if len(userIds) == 0 {
		return nil, nil
	}
	users, _, err := apiClient.GetUsersByIds(ctx, userIds)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*userSummary, len(users))
	for _, user := range users {
		result[user.Id] = &userSummary{
			ID:        user.Id,
			Username:  user.Username,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Nickname:  user.Nickname,
			Position:  user.Position,
		}
	}
	return result, nil
}

func buildUserCache(users map[string]*userSummary) map[string]string {
	cache := make(map[string]string, len(users))
	for id, user := range users {
		cache[id] = user.Username
	}
	return cache
}

func printPostListWithUsers(ctx context.Context, apiClient *model.Client4, postList *model.PostList) {
	userIds := collectUserIdsFromPostList(postList)
	users, _ := resolveUsersByIds(ctx, apiClient, userIds)

	raw, _ := json.Marshal(postList)
	var combined map[string]interface{}
	_ = json.Unmarshal(raw, &combined)
	if users != nil {
		combined["users"] = users
	}
	printer.PrintJSON(combined)
}

func printPostsWithUsers(ctx context.Context, apiClient *model.Client4, posts []*model.Post) {
	userIds := collectUserIdsFromPosts(posts)
	users, _ := resolveUsersByIds(ctx, apiClient, userIds)
	printer.PrintJSON(map[string]interface{}{
		"posts": posts,
		"users": users,
	})
}

// resolveTeamId returns the team ID from the --team flag, the active profile, or an error.
func resolveTeamId(ctx context.Context, command *cobra.Command, apiClient *model.Client4, server *config.ServerProfile) (string, error) {
	teamOverride, _ := command.Flags().GetString("team")
	if teamOverride != "" {
		team, _, err := apiClient.GetTeamByName(ctx, teamOverride, "")
		if err != nil {
			return "", fmt.Errorf("team %q not found: %w", teamOverride, err)
		}
		return team.Id, nil
	}
	if server.TeamID != "" {
		return server.TeamID, nil
	}
	return "", fmt.Errorf("no active team set. Use --team <name> or run: mm team switch <name>")
}
