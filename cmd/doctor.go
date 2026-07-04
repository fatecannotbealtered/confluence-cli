package cmd

import (
	"fmt"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/config"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration and connectivity step by step",
	Args:  cobra.NoArgs,
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type doctorCheck struct {
	Check   string  `json:"check"`
	Status  string  `json:"status"`
	Message string  `json:"message,omitempty"`
	Fix     *string `json:"fix"`
}

type doctorResult struct {
	Checks        []doctorCheck `json:"checks"`
	URL           string        `json:"url,omitempty"`
	Username      string        `json:"username,omitempty"`
	DisplayName   string        `json:"display_name,omitempty"`
	ServerVersion string        `json:"server_version,omitempty"`
	LatencyMs     int64         `json:"latency_ms,omitempty"`
}

func runDoctor(_ *cobra.Command, _ []string) error {
	var result doctorResult
	addCheck := func(check, status, msg, fix string) {
		var fixPtr *string
		if fix != "" {
			fixPtr = &fix
		}
		result.Checks = append(result.Checks, doctorCheck{Check: check, Status: status, Message: msg, Fix: fixPtr})
	}
	addReleaseReadinessCheck := func() {
		addCheck("release_readiness", releaseReadinessCheckStatus(), buildReleaseReadiness().Reason, releaseReadinessCheckFix())
	}
	finishFail := func() error {
		addReleaseReadinessCheck()
		if jsonMode {
			output.PrintJSON(result)
		} else {
			printDoctorText(result)
		}
		return SilentErr(ExitAuth)
	}

	// Step 1: config exists and parses.
	cfg, err := config.Load()
	if err != nil {
		addCheck("config", "fail", err.Error(), "fix or remove "+config.FilePath())
		return finishFail()
	}
	if cfg.URL == "" || cfg.Token == "" {
		addCheck("config", "fail", "Confluence URL/token not configured",
			"run 'confluence-cli auth login --url <url> --token <pat>' or set "+config.EnvURL+" and "+config.EnvToken)
		return finishFail()
	}
	addCheck("config", "pass", "configuration found", "")
	result.URL = cfg.URL

	// Step 2+3: URL reachable and PAT valid (GET /rest/api/user/current probes both).
	client := api.NewClient(cfg.URL, cfg.Token, api.Options{Version: version})
	start := time.Now()
	me, err := client.Users.CurrentUser()
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		addCheck("network", "fail", err.Error(), "verify the base URL is reachable; set HTTP_PROXY/HTTPS_PROXY if required")
		addCheck("auth", "fail", err.Error(), "check PAT validity in Confluence Profile > Personal Access Tokens")
		return finishFail()
	}
	addCheck("network", "pass", fmt.Sprintf("connected in %dms", result.LatencyMs), "")
	addCheck("auth", "pass", "PAT valid", "")
	result.Username = me.Username
	result.DisplayName = me.DisplayName

	// Step 4: server version (settings/systemInfo). Best effort: some DC
	// versions do not serve this endpoint (404) or restrict it to admins. Since
	// the auth step already proved the server is reachable and the PAT is valid,
	// an unreadable version is informational, not a warning — report pass with
	// the version omitted rather than flagging a healthy instance.
	if info, err := client.System.SystemInfo(); err != nil {
		addCheck("server", "pass", "connected; server version endpoint unavailable on this instance",
			"")
	} else {
		result.ServerVersion = info.Version
		addCheck("server", "pass", "Confluence server version "+info.Version, "")
	}

	// Step 5: release readiness declaration.
	addReleaseReadinessCheck()

	if jsonMode {
		output.PrintJSON(result)
		return nil
	}
	printDoctorText(result)
	return nil
}

func printDoctorText(result doctorResult) {
	fmt.Println()
	output.Bold("  confluence-cli Doctor")
	output.Gray("  ────────────────────────────────────────")
	fmt.Println()
	for _, c := range result.Checks {
		switch c.Status {
		case "pass":
			output.Success(c.Check + ": " + c.Message)
		case "warn":
			output.Warn(c.Check + ": " + c.Message)
		default:
			output.Error(c.Check + ": " + c.Message)
			if c.Fix != nil {
				output.Gray("  fix: " + *c.Fix)
			}
		}
	}
	if result.DisplayName != "" {
		output.Gray(fmt.Sprintf("  Authenticated as %s (%s)", result.DisplayName, result.Username))
	}
	if result.LatencyMs > 0 {
		output.Gray(fmt.Sprintf("  Latency: %dms", result.LatencyMs))
	}
	fmt.Println()
}
