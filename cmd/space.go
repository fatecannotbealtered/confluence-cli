package cmd

import (
	"strings"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	spaceListType   string
	spaceListLimit  int
	spaceListStart  int
	spaceCreateKey  string
	spaceCreateName string
	spaceCreateDesc string
	spaceUpdateName string
	spaceUpdateDesc string
	spaceDeleteWait bool
	spaceDeleteTO   int
)

var spaceCmd = &cobra.Command{
	Use:   "space",
	Short: "Manage Confluence spaces",
}

var spaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List spaces",
	Args:  cobra.NoArgs,
	RunE:  runSpaceList,
}

var spaceGetCmd = &cobra.Command{
	Use:   "get <KEY>",
	Short: "Get a space by key",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpaceGet,
}

var spaceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a space",
	Args:  cobra.NoArgs,
	RunE:  runSpaceCreate,
}

var spaceUpdateCmd = &cobra.Command{
	Use:   "update <KEY>",
	Short: "Update a space's name/description",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpaceUpdate,
}

var spaceDeleteCmd = &cobra.Command{
	Use:   "delete <KEY>",
	Short: "Delete a space (asynchronous, write-dangerous)",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpaceDelete,
}

func init() {
	spaceListCmd.Flags().StringVar(&spaceListType, "type", "", "Filter by space type: global or personal")
	spaceListCmd.Flags().IntVar(&spaceListLimit, "limit", 0, "Maximum number of spaces to return")
	spaceListCmd.Flags().IntVar(&spaceListStart, "start-at", 0, "Zero-based offset of the first result")

	spaceCreateCmd.Flags().StringVar(&spaceCreateKey, "key", "", "Space key (required)")
	spaceCreateCmd.Flags().StringVar(&spaceCreateName, "name", "", "Space name (required)")
	spaceCreateCmd.Flags().StringVar(&spaceCreateDesc, "description", "", "Space description")
	markWrite(spaceCreateCmd)

	spaceUpdateCmd.Flags().StringVar(&spaceUpdateName, "name", "", "New space name")
	spaceUpdateCmd.Flags().StringVar(&spaceUpdateDesc, "description", "", "New space description")
	markWrite(spaceUpdateCmd)

	spaceDeleteCmd.Flags().BoolVar(&spaceDeleteWait, "wait", false, "Poll the delete long task until it finishes")
	spaceDeleteCmd.Flags().IntVar(&spaceDeleteTO, "timeout", 60, "Seconds to wait when --wait is set")
	markWrite(spaceDeleteCmd)

	spaceCmd.AddCommand(spaceListCmd, spaceGetCmd, spaceCreateCmd, spaceUpdateCmd, spaceDeleteCmd)
	rootCmd.AddCommand(spaceCmd)

	dangerousCommandPaths["confluence-cli space delete"] = true
}

// flatSpace projects an api.Space onto the token-efficient FlatSpace shape,
// tagging name/description as untrusted external content.
func flatSpace(s *api.Space) output.FlatSpace {
	fs := output.FlatSpace{
		Key:       s.Key,
		Name:      s.Name,
		Type:      s.Type,
		Status:    s.Status,
		Untrusted: []string{"name"},
	}
	if s.Description != nil {
		fs.Description = s.Description.Plain.Value
		if fs.Description != "" {
			fs.Untrusted = append(fs.Untrusted, "description")
		}
	}
	return fs
}

func runSpaceList(_ *cobra.Command, _ []string) error {
	if spaceListType != "" && spaceListType != "global" && spaceListType != "personal" {
		emitError(output.ErrValidation, "--type must be global or personal", nil)
		return SilentErr(ExitBadArgs)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Spaces.ListSpaces(spaceListType, spaceListStart, spaceListLimit)
	if err != nil {
		return emitAPIError(err)
	}
	items := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, output.FilterSpaceFields(flatSpace(&page.Items[i]), fieldsList))
	}
	output.PrintJSON(output.PagedMap(items, len(items), page.Start, page.NextStartAt, page.HasMore))
	return nil
}

func runSpaceGet(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	s, err := client.Spaces.GetSpace(args[0])
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(output.FilterSpaceFields(flatSpace(s), fieldsList))
	return nil
}

func runSpaceCreate(_ *cobra.Command, _ []string) error {
	key := strings.TrimSpace(spaceCreateKey)
	name := strings.TrimSpace(spaceCreateName)
	if key == "" || name == "" {
		emitError(output.ErrValidation, "--key and --name are required", nil)
		return SilentErr(ExitBadArgs)
	}
	if dryRunOutput("space create", map[string]any{"key": key, "name": name}) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	req := &api.CreateSpaceRequest{Key: key, Name: name}
	if spaceCreateDesc != "" {
		req.Description = &api.SpaceDescription{Plain: api.BodyRepresentation{Value: spaceCreateDesc, Representation: "plain"}}
	}
	s, err := client.Spaces.CreateSpace(req)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(output.FilterSpaceFields(flatSpace(s), fieldsList))
	return nil
}

func runSpaceUpdate(_ *cobra.Command, args []string) error {
	key := args[0]
	if spaceUpdateName == "" && spaceUpdateDesc == "" {
		emitError(output.ErrValidation, "provide --name and/or --description", nil)
		return SilentErr(ExitBadArgs)
	}
	updateDetail := map[string]any{"key": key}
	if spaceUpdateName != "" {
		updateDetail["name"] = spaceUpdateName
	}
	if spaceUpdateDesc != "" {
		updateDetail["description"] = spaceUpdateDesc
	}
	// Bind name/description into the confirm token (CLI-SPEC §7) so the token
	// cannot authorize an arbitrary payload different from the preview.
	if dryRunOutput("space update", updateDetail) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	req := &api.UpdateSpaceRequest{Name: spaceUpdateName}
	if spaceUpdateDesc != "" {
		req.Description = &api.SpaceDescription{Plain: api.BodyRepresentation{Value: spaceUpdateDesc, Representation: "plain"}}
	}
	s, err := client.Spaces.UpdateSpace(key, req)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(output.FilterSpaceFields(flatSpace(s), fieldsList))
	return nil
}

func runSpaceDelete(_ *cobra.Command, args []string) error {
	key := args[0]
	if dryRunOutput("space delete", map[string]any{"key": key}) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	lt, err := client.Spaces.DeleteSpace(key)
	if err != nil {
		return emitAPIError(err)
	}
	result := map[string]any{
		"task_id":     lt.ID,
		"status":      "in_progress",
		"status_link": lt.Links.Status,
	}
	if !spaceDeleteWait {
		output.PrintJSON(result)
		return nil
	}
	return waitForSpaceDelete(client, lt.ID, result)
}

// waitForSpaceDelete polls GetLongTask until the task finishes or the timeout
// elapses. On timeout it emits E_TIMEOUT while preserving task_id in details.
func waitForSpaceDelete(client *api.Client, taskID string, result map[string]any) error {
	deadline := clockNow().Add(time.Duration(spaceDeleteTO) * time.Second)
	for {
		task, err := client.LongTasks.GetLongTask(taskID)
		if err != nil {
			return emitAPIError(err)
		}
		if task.Finished {
			result["status"] = "finished"
			result["successful"] = task.Successful
			result["percentage_complete"] = task.PercentageComplete
			output.PrintJSON(result)
			if !task.Successful {
				return SilentErr(ExitGeneric)
			}
			return nil
		}
		if !clockNow().Before(deadline) {
			emitError(output.ErrTimeout, "timed out waiting for space delete to finish", map[string]any{"task_id": taskID})
			return SilentErr(ExitTimeout)
		}
		clockSleep(pollInterval)
	}
}
