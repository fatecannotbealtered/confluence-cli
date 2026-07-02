package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/audit"
	"github.com/fatecannotbealtered/confluence-cli/internal/confirm"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

// Exit codes for machine-readable error classification (contract table).
const (
	ExitOK              = 0
	ExitGeneric         = 1
	ExitBadArgs         = 2
	ExitNotFound        = 3
	ExitAuth            = 4
	ExitConfirmRequired = 5
	ExitConflict        = 6
	ExitRetryable       = 7
	ExitTimeout         = 8
	ExitInterrupted     = 130
)

// ErrSilent indicates the error has been printed; cobra should not print again.
var ErrSilent = errors.New("")

// version is injected by goreleaser/Makefile ldflags
// (-X github.com/fatecannotbealtered/confluence-cli/cmd.version=...).
var version = "dev"

const (
	outputFormatJSON = "json"
	outputFormatText = "text"
	outputFormatRaw  = "raw"
)

// Global flags (fleet-aligned set).
var (
	outputFormat  = outputFormatJSON
	compactMode   bool
	fieldsList    []string
	quietMode     bool
	dryRun        bool
	confirmToken  string
	dangerousMode bool
)

// jsonMode reports whether command success output should use the machine-readable branch.
var jsonMode = true

// dangerousCommandPaths is the single source of truth for which command paths
// are write-dangerous. Both permissionTier (self-description) and the runtime
// gate read it, so the advertised tier can never drift from what is enforced.
// Empty in this phase: no irreversible/bulk Confluence-data command exists yet.
var dangerousCommandPaths = map[string]bool{}

// lastExit tracks the exit code for the current command execution.
var lastExit int

// cmdStartTime records when the current command began (for audit logging).
var cmdStartTime time.Time

// execCmd tracks the leaf command during Execute for post-run audit logging.
var execCmd *cobra.Command

// LastExitCode returns the exit code from the last command execution.
func LastExitCode() int { return lastExit }

// setExitCode sets the exit code (only increases severity, never decreases).
func setExitCode(code int) {
	if code > lastExit {
		lastExit = code
	}
}

// SilentErr sets the exit code and returns ErrSilent so cobra does not print again.
func SilentErr(code int) error {
	setExitCode(code)
	return ErrSilent
}

var rootCmd = &cobra.Command{
	Use:           "confluence-cli",
	Short:         "Confluence Data Center CLI for AI Agents",
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: fmt.Sprintf("\n  %s\n  %s",
		output.FormatCyanBold("confluence-cli"),
		output.FormatGray("Agent-native Confluence Data Center control")),
}

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
	}
	rootCmd.Version = version
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", outputFormatJSON, "Output format: json, text, or raw")
	rootCmd.PersistentFlags().BoolVar(&compactMode, "compact", false, "Emit compact JSON (only with --format json)")
	rootCmd.PersistentFlags().StringSliceVar(&fieldsList, "fields", nil, "Restrict JSON data to these fields (only with --format json)")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress auxiliary text output (does not suppress json/raw results)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without executing")
	rootCmd.PersistentFlags().StringVar(&confirmToken, "confirm", "", "Confirmation token returned by --dry-run for write commands")
	rootCmd.PersistentFlags().BoolVar(&dangerousMode, "dangerous", false, "Required for write-dangerous commands in both the dry-run and confirm steps")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		lastExit = 0
		cmdStartTime = time.Now()
		output.CommandStartTime = cmdStartTime
		execCmd = cmd
		return configureOutputMode(cmd)
	}
}

func configureOutputMode(cmd *cobra.Command) error {
	effectiveFormat := strings.ToLower(strings.TrimSpace(outputFormat))
	if effectiveFormat == "" {
		effectiveFormat = outputFormatJSON
	}
	switch effectiveFormat {
	case outputFormatJSON, outputFormatText, outputFormatRaw:
	default:
		return formatArgumentError(outputFormatJSON, "--format must be one of: json, text, raw")
	}
	if compactMode && effectiveFormat != outputFormatJSON {
		return formatArgumentError(effectiveFormat, "--compact can only be used with --format json")
	}
	if persistentFlagChanged(cmd, "fields") && effectiveFormat != outputFormatJSON {
		return formatArgumentError(effectiveFormat, "--fields can only be used with --format json")
	}
	if effectiveFormat == outputFormatRaw && !supportsRawFormat(cmd) {
		return formatArgumentError(effectiveFormat, cmd.CommandPath()+" does not support --format raw")
	}

	outputFormat = effectiveFormat
	jsonMode = effectiveFormat != outputFormatText
	output.CompactJSON = compactMode && effectiveFormat == outputFormatJSON
	output.EnvelopeJSON = effectiveFormat == outputFormatJSON
	output.ErrorJSON = effectiveFormat != outputFormatText
	output.Quiet = effectiveFormat != outputFormatText || quietMode
	return nil
}

func persistentFlagChanged(cmd *cobra.Command, name string) bool {
	f := cmd.Root().PersistentFlags().Lookup(name)
	return f != nil && f.Changed
}

func formatArgumentError(format, msg string) error {
	if format == outputFormatText {
		output.ErrorJSON = false
		output.Error(msg)
	} else {
		output.CompactJSON = compactMode
		output.PrintErrorJSONWithCode(msg, 0, output.ErrValidation)
	}
	return SilentErr(ExitBadArgs)
}

// Execute runs the root command with a background context.
func Execute() error {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the root command with the given context
// (e.g. signal.NotifyContext for SIGINT/SIGTERM).
func ExecuteContext(ctx context.Context) error {
	lastExit = 0
	execCmd = nil
	cmdStartTime = time.Now()
	output.CommandStartTime = cmdStartTime
	err := rootCmd.ExecuteContext(ctx)
	if err != nil && !errors.Is(err, ErrSilent) {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			err = emitInterrupted()
		} else {
			err = handleCommandError(err)
		}
	}
	if err == nil && lastExit != ExitOK {
		err = ErrSilent
	}
	if execCmd != nil {
		audit.Log(execCmd.CommandPath(), os.Args[1:], lastExit, time.Since(cmdStartTime).Milliseconds())
	}
	return err
}

// emitInterrupted writes the terminal E_INTERRUPTED envelope (exit 130) so an
// interrupted run still hands the agent a parseable terminal document.
func emitInterrupted() error {
	msg := "operation cancelled by signal"
	if errorOutputFormat() == outputFormatText {
		output.ErrorJSON = false
		output.Error(msg)
	} else {
		output.CompactJSON = compactMode
		output.PrintErrorJSONWithCode(msg, 0, output.ErrInterrupted)
	}
	return SilentErr(ExitInterrupted)
}

// handleCommandError reports cobra-level errors (unknown flags/args) as
// E_VALIDATION in the active output format.
func handleCommandError(err error) error {
	if errorOutputFormat() == outputFormatText {
		output.ErrorJSON = false
		output.Error(err.Error())
	} else {
		output.CompactJSON = compactMode
		output.PrintErrorJSONWithCode(err.Error(), 0, output.ErrValidation)
	}
	return SilentErr(ExitBadArgs)
}

func errorOutputFormat() string {
	format := strings.ToLower(strings.TrimSpace(outputFormat))
	if format == outputFormatText {
		return outputFormatText
	}
	return outputFormatJSON
}

// dryRunOutput implements the write gate: with --dry-run it prints the preview
// plus a confirm token and returns true (skip execution); without --dry-run in
// JSON mode it enforces and consumes the --confirm token, returning true when
// the write must NOT proceed.
func dryRunOutput(action string, detail map[string]any) bool {
	if detail == nil {
		detail = map[string]any{}
	}
	// T2 second gate: write-dangerous commands require --dangerous in BOTH the
	// dry-run and confirm steps, on top of the --confirm token.
	if isDangerousCommand(execCmd) && !dangerousMode {
		msg := currentCommandPath() + " is write-dangerous and requires --dangerous in both the --dry-run and --confirm steps"
		if jsonMode {
			output.PrintErrorJSONWithCode(msg, 0, output.ErrConfirmRequired)
		} else {
			output.Error(msg)
		}
		setExitCode(ExitConfirmRequired)
		return true
	}
	if jsonMode {
		if dryRun {
			token, expiresAt := confirm.Issue(action, detail)
			output.PrintJSON(map[string]any{
				"preview": map[string]any{
					"action":  action,
					"changes": []map[string]any{{"operation": action, "target": detail}},
				},
				"confirm_token": token,
				"expires_at":    expiresAt.Format(time.RFC3339),
			})
			return true
		}
		if execCmd != nil && isWriteCommand(execCmd) {
			if confirmToken == "" {
				output.PrintErrorJSONWithCode("write command requires --confirm token; run with --dry-run first", 0, output.ErrConfirmRequired)
				setExitCode(ExitConfirmRequired)
				return true
			}
			if err := confirm.Consume(action, detail, confirmToken, time.Now().UTC()); err != nil {
				output.PrintErrorJSONWithCode(err.Error(), 0, output.ErrConflict)
				setExitCode(ExitConflict)
				return true
			}
		}
	} else if dryRun {
		output.Info("[dry-run] " + action)
		return true
	}
	return false
}

func currentCommandPath() string {
	if execCmd != nil {
		return execCmd.CommandPath()
	}
	return ""
}

// isWriteCommand returns true if the command has the "write" annotation.
func isWriteCommand(cmd *cobra.Command) bool {
	return cmd.Annotations["write"] == "true"
}

// isDangerousCommand reports whether the command is in the write-dangerous set
// that requires the --dangerous second gate.
func isDangerousCommand(cmd *cobra.Command) bool {
	return cmd != nil && dangerousCommandPaths[cmd.CommandPath()]
}

// markWrite sets the "write" annotation on a command (confirm gate + audit tier).
func markWrite(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["write"] = "true"
}

func supportsRawFormat(cmd *cobra.Command) bool {
	return cmd.Annotations["format.raw"] == "true"
}
