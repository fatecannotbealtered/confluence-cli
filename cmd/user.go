package cmd

import (
	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	userSearchLimit int
	userSearchStart int
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Look up Confluence users",
}

var userCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the authenticated user",
	Args:  cobra.NoArgs,
	RunE:  runUserCurrent,
}

var userGetCmd = &cobra.Command{
	Use:   "get <USERNAME>",
	Short: "Get a user by username",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserGet,
}

var userSearchCmd = &cobra.Command{
	Use:   "search <QUERY>",
	Short: "Search users by name",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserSearch,
}

func init() {
	userSearchCmd.Flags().IntVar(&userSearchLimit, "limit", 0, "Maximum number of users to return")
	userSearchCmd.Flags().IntVar(&userSearchStart, "start-at", 0, "Zero-based offset of the first result")
	userCmd.AddCommand(userCurrentCmd, userGetCmd, userSearchCmd)
	rootCmd.AddCommand(userCmd)
}

// userMap projects a user onto the output shape, tagging display_name untrusted.
func userMap(u *api.User) map[string]any {
	return map[string]any{
		"username":     u.Username,
		"display_name": u.DisplayName,
		"user_key":     u.UserKey,
		"type":         u.Type,
		"_untrusted":   []string{"display_name"},
	}
}

func runUserCurrent(_ *cobra.Command, _ []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	u, err := client.Users.CurrentUser()
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(userMap(u))
	return nil
}

func runUserGet(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	u, err := client.Users.GetUser(args[0])
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(userMap(u))
	return nil
}

func runUserSearch(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Search.SearchUser(args[0], api.SearchOptions{Start: userSearchStart, Limit: userSearchLimit})
	if err != nil {
		return emitAPIError(err)
	}
	users := make([]map[string]any, 0, len(page.Results))
	for i := range page.Results {
		if u := page.Results[i].User; u != nil {
			users = append(users, userMap(u))
		}
	}
	result := output.PagedMap(users, len(users), page.Start, page.NextStart, page.HasMore)
	result["total_size"] = page.TotalSize
	output.PrintJSON(result)
	return nil
}
