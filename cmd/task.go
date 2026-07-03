package cmd

import (
	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Inspect Confluence long-running tasks",
}

var taskGetCmd = &cobra.Command{
	Use:   "get <TASK_ID>",
	Short: "Get the status of a long-running task",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskGet,
}

func init() {
	taskCmd.AddCommand(taskGetCmd)
	rootCmd.AddCommand(taskCmd)
}

func runTaskGet(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	task, err := client.LongTasks.GetLongTask(args[0])
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(taskMap(task))
	return nil
}

// taskMap projects a long task onto the output shape.
func taskMap(t *api.LongTask) map[string]any {
	name := ""
	if t.Name != nil {
		name = t.Name.Key
	}
	messages := make([]string, 0, len(t.Messages))
	for _, m := range t.Messages {
		messages = append(messages, m.Translation)
	}
	return map[string]any{
		"id":                  t.ID,
		"name":                name,
		"percentage_complete": t.PercentageComplete,
		"successful":          t.Successful,
		"finished":            t.Finished,
		"messages":            messages,
	}
}
