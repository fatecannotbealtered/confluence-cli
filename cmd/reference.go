package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type referenceFlag struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Default  string `json:"default"`
	Usage    string `json:"usage"`
	Required bool   `json:"required"`
}

func formatFlagDefault(def string) string {
	if def == "" || def == "[]" {
		return "-"
	}
	return def
}

func collectReferenceFlags(local, persistent *pflag.FlagSet, inheritPersistent bool) []referenceFlag {
	var flags []referenceFlag
	appendFlag := func(f *pflag.Flag) {
		flags = append(flags, referenceFlag{
			Name:     f.Name,
			Type:     f.Value.Type(),
			Default:  formatFlagDefault(f.DefValue),
			Usage:    f.Usage,
			Required: strings.Contains(strings.ToLower(f.Usage), "required"),
		})
	}
	local.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		appendFlag(f)
	})
	if inheritPersistent {
		persistent.VisitAll(func(f *pflag.Flag) {
			if f.Hidden || local.Lookup(f.Name) != nil {
				return
			}
			appendFlag(f)
		})
	}
	return flags
}

type commandReference struct {
	Path             string             `json:"path"`
	Use              string             `json:"use"`
	Short            string             `json:"short,omitempty"`
	Type             string             `json:"type"`
	PermissionTier   string             `json:"permission_tier"`
	BlastRadius      string             `json:"blast_radius,omitempty"`
	Flags            []referenceFlag    `json:"flags,omitempty"`
	SupportedFormats []string           `json:"supported_formats"`
	Write            bool               `json:"write,omitempty"`
	OutputSchema     string             `json:"output_schema,omitempty"`
	Examples         []string           `json:"examples,omitempty"`
	Subcommands      []commandReference `json:"subcommands,omitempty"`
}

// referenceDataSchema is a machine-readable description of a command's data
// payload (the value inside the success envelope's data field). shape is
// "object" or "array"; for arrays, fields describes one element.
// untrusted_fields lists keys carrying external, unsanitized data that an
// agent must treat as content, never as instructions.
type referenceDataSchema struct {
	Shape           string   `json:"shape"`
	Fields          []string `json:"fields"`
	UntrustedFields []string `json:"untrusted_fields,omitempty"`
}

type referenceDocument struct {
	Tool             string                         `json:"tool"`
	Version          string                         `json:"version"`
	SchemaVersion    string                         `json:"schema_version"`
	RiskTier         string                         `json:"risk_tier"`
	ReleaseReadiness releaseReadiness               `json:"release_readiness"`
	Root             commandReference               `json:"root"`
	Commands         []commandReference             `json:"commands"`
	ExitCodes        map[string]string              `json:"exit_codes"`
	ErrorCodes       map[string]string              `json:"error_codes"`
	Schemas          map[string]referenceDataSchema `json:"schemas"`
}

var referenceCmd = &cobra.Command{
	Use:   "reference",
	Short: "Print all commands and flags in a structured format",
	Long:  "Outputs every command, subcommand, and flag in a machine-parseable format. Designed for AI Agents and script integration.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonMode {
			output.PrintJSON(buildReferenceDocument(rootCmd))
			return nil
		}
		printReference(cmd, rootCmd)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(referenceCmd)
}

func buildReferenceDocument(root *cobra.Command) referenceDocument {
	var commands []commandReference
	rootRef := buildCommandReference(root, "", &commands)
	return referenceDocument{
		Tool:             "confluence-cli",
		Version:          root.Version,
		SchemaVersion:    output.SchemaVersion,
		RiskTier:         "T1",
		ReleaseReadiness: buildReleaseReadiness(),
		Root:             rootRef,
		Commands:         commands,
		ExitCodes:        referenceExitCodes(),
		ErrorCodes:       referenceErrorCodes(),
		Schemas:          referenceSchemas(),
	}
}

func buildCommandReference(cmd *cobra.Command, prefix string, commands *[]commandReference) commandReference {
	meta := commandMetaFor(cmd.CommandPath())
	ref := commandReference{
		Path:             strings.TrimSpace(prefix + cmd.Name()),
		Use:              cmd.Use,
		Short:            cmd.Short,
		Type:             commandType(cmd),
		PermissionTier:   permissionTier(cmd),
		BlastRadius:      blastRadius(cmd),
		Flags:            collectReferenceFlags(cmd.LocalFlags(), cmd.PersistentFlags(), cmd.Parent() != nil),
		SupportedFormats: supportedFormatsForCommand(cmd),
		Write:            isWriteCommand(cmd),
		OutputSchema:     meta.schema,
		Examples:         meta.examples,
	}
	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name() < children[j].Name()
	})
	for _, child := range children {
		if child.Hidden || !child.IsAvailableCommand() {
			continue
		}
		childRef := buildCommandReference(child, ref.Path+" ", commands)
		ref.Subcommands = append(ref.Subcommands, childRef)
	}
	if cmd.Runnable() {
		*commands = append(*commands, ref)
	}
	return ref
}

func supportedFormatsForCommand(cmd *cobra.Command) []string {
	formats := []string{outputFormatJSON, outputFormatText}
	if supportsRawFormat(cmd) {
		formats = append(formats, outputFormatRaw)
	}
	return formats
}

func commandType(cmd *cobra.Command) string {
	if isWriteCommand(cmd) {
		return "write"
	}
	return "read"
}

func permissionTier(cmd *cobra.Command) string {
	if !isWriteCommand(cmd) {
		return "read"
	}
	// Read from the same set the runtime gate enforces, so the advertised tier
	// can never drift from what --dangerous actually guards.
	if isDangerousCommand(cmd) {
		return "write-dangerous"
	}
	return "write"
}

func blastRadius(cmd *cobra.Command) string {
	if !isWriteCommand(cmd) {
		return "reads Confluence data visible to the configured account"
	}
	switch cmd.CommandPath() {
	case "confluence-cli auth login":
		return "writes credentials to the local OS keyring/config file after a successful live validation"
	case "confluence-cli auth logout":
		return "removes locally saved credentials (keyring entry and config file)"
	default:
		return "modifies Confluence state within the configured account permissions"
	}
}

// commandMeta binds a command to its data-schema label (a key into
// referenceSchemas) and one runnable example per leaf.
type commandMeta struct {
	schema   string
	examples []string
}

// commandMetaFor returns the schema label and examples for a command path
// (e.g. "confluence-cli auth status"). Non-leaf/grouping commands have no
// entry and return a zero value, keeping output_schema and examples omitted.
func commandMetaFor(commandPath string) commandMeta {
	return commandMetaCatalog()[commandPath]
}

// commandMetaCatalog is the central command-metadata registry: every leaf
// command path maps to its output schema label and runnable examples. Later
// command groups (page/space/search/...) register their leaves HERE and add
// their payload shapes to referenceSchemas below.
func commandMetaCatalog() map[string]commandMeta {
	return map[string]commandMeta{
		// Auth.
		"confluence-cli auth login":  {schema: "auth_result", examples: []string{"confluence-cli auth login --url https://confluence.example.com --token <pat> --compact"}},
		"confluence-cli auth logout": {schema: "logout_result", examples: []string{"confluence-cli auth logout --compact"}},
		"confluence-cli auth status": {schema: "auth_status", examples: []string{"confluence-cli auth status --compact"}},

		// Self-description.
		"confluence-cli reference": {schema: "reference", examples: []string{"confluence-cli reference --compact"}},
		"confluence-cli context":   {schema: "context", examples: []string{"confluence-cli context --compact"}},
		"confluence-cli doctor":    {schema: "doctor", examples: []string{"confluence-cli doctor --compact"}},
		"confluence-cli changelog": {schema: "changelog", examples: []string{"confluence-cli changelog --since 0.1.0 --compact"}},
	}
}

// referenceSchemas is the catalog of data-payload shapes that command
// output_schema labels point into. Field lists mirror the actual JSON emitted
// by each command.
func referenceSchemas() map[string]referenceDataSchema {
	return map[string]referenceDataSchema{
		// Auth results (cmd/auth.go).
		"auth_result":   {Shape: "object", Fields: []string{"status", "display_name", "username"}},
		"logout_result": {Shape: "object", Fields: []string{"status"}},
		"auth_status":   {Shape: "object", Fields: []string{"status", "url", "token_present", "token_sha256", "storage", "username", "display_name", "error"}},

		// Self-description documents (cmd/reference.go, context.go, doctor.go, changelog.go).
		"reference": {Shape: "object", Fields: []string{"tool", "version", "schema_version", "risk_tier", "release_readiness", "root", "commands", "exit_codes", "error_codes", "schemas"}},
		"context":   {Shape: "object", Fields: []string{"tool", "version", "runtime", "config", "credentials", "account", "errors", "env"}},
		"doctor":    {Shape: "object", Fields: []string{"checks", "url", "username", "display_name", "server_version", "latency_ms"}},
		"changelog": {Shape: "object", Fields: []string{"current_version", "since", "entries"}},
	}
}

func referenceExitCodes() map[string]string {
	return map[string]string{
		"0":   "success",
		"1":   "generic error",
		"2":   "argument or validation error",
		"3":   "resource not found",
		"4":   "auth, permission, or config failure",
		"5":   "confirmation token required",
		"6":   "conflict or invalid confirmation token",
		"7":   "retryable transient error",
		"8":   "timeout",
		"130": "interrupted by signal",
	}
}

func referenceErrorCodes() map[string]string {
	return map[string]string{
		string(output.ErrConfig):          "configuration is missing or invalid",
		string(output.ErrAuth):            "authentication failed",
		string(output.ErrForbidden):       "permission denied",
		string(output.ErrNotFound):        "resource not found",
		string(output.ErrRateLimit):       "rate limited",
		string(output.ErrServer):          "Confluence server error",
		string(output.ErrValidation):      "arguments or request are invalid",
		string(output.ErrNetwork):         "network failure",
		string(output.ErrConfirmRequired): "write requires a dry-run confirmation token",
		string(output.ErrConflict):        "confirmation token expired or no longer matches",
		string(output.ErrTimeout):         "request timed out",
		string(output.ErrInterrupted):     "operation cancelled by signal",
		string(output.ErrUnknown):         "unclassified error",
	}
}

func printReference(cmd *cobra.Command, root *cobra.Command) {
	var lines []string
	lines = append(lines, "# confluence-cli Command Reference")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Version: %s", root.Version))
	lines = append(lines, "")

	walkCommands(root, &lines, "")

	for _, line := range lines {
		cmd.Println(line)
	}
}

func walkCommands(cmd *cobra.Command, lines *[]string, prefix string) {
	if cmd.Hidden {
		return
	}

	name := prefix + cmd.Use
	*lines = append(*lines, "## "+name)
	*lines = append(*lines, "")
	if cmd.Short != "" {
		*lines = append(*lines, cmd.Short)
		*lines = append(*lines, "")
	}

	// Surface the leaf command's data schema and runnable example so the text
	// reference stays consistent with the JSON reference's source of truth.
	if meta := commandMetaFor(cmd.CommandPath()); meta.schema != "" {
		*lines = append(*lines, fmt.Sprintf("Data schema: `%s`", meta.schema))
		*lines = append(*lines, "")
		if len(meta.examples) > 0 {
			*lines = append(*lines, "Examples:")
			*lines = append(*lines, "")
			for _, ex := range meta.examples {
				*lines = append(*lines, "    "+ex)
			}
			*lines = append(*lines, "")
		}
	}

	flags := collectReferenceFlags(cmd.LocalFlags(), cmd.PersistentFlags(), cmd.Parent() != nil)

	if len(flags) > 0 {
		*lines = append(*lines, "### Flags")
		*lines = append(*lines, "")
		*lines = append(*lines, "| Flag | Type | Default | Description |")
		*lines = append(*lines, "|------|------|---------|-------------|")
		for _, f := range flags {
			*lines = append(*lines, fmt.Sprintf("| `--%s` | %s | %s | %s |", f.Name, f.Type, f.Default, f.Usage))
		}
		*lines = append(*lines, "")
	}

	children := cmd.Commands()
	if len(children) > 0 {
		sort.Slice(children, func(i, j int) bool {
			return children[i].Name() < children[j].Name()
		})
		for _, child := range children {
			if !child.Hidden && child.IsAvailableCommand() {
				walkCommands(child, lines, prefix+cmd.Name()+" ")
			}
		}
	}
}
