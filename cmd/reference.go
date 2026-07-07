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
		RiskTier:         "T2",
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
		"confluence-cli update":    {schema: "update_result", examples: []string{"confluence-cli update --check --compact", "confluence-cli update --dry-run --compact"}},

		// Space.
		"confluence-cli space list":   {schema: "space_list", examples: []string{"confluence-cli space list --type global --limit 25 --compact"}},
		"confluence-cli space get":    {schema: "space", examples: []string{"confluence-cli space get ENG --compact"}},
		"confluence-cli space create": {schema: "space", examples: []string{"confluence-cli space create --key ENG --name Engineering --dry-run"}},
		"confluence-cli space update": {schema: "space", examples: []string{"confluence-cli space update ENG --name 'Engineering Team' --dry-run"}},
		"confluence-cli space delete": {schema: "space_delete", examples: []string{"confluence-cli space delete ENG --dangerous --dry-run"}},

		// Search.
		"confluence-cli search": {schema: "search_result", examples: []string{
			"confluence-cli search --type page --space ENG --title roadmap --sort modified --desc --compact",
			`confluence-cli search 'label = "adr"' --space ENG --modified-since -30d --compact`,
		}},

		// User.
		"confluence-cli user current": {schema: "user", examples: []string{"confluence-cli user current --compact"}},
		"confluence-cli user get":     {schema: "user", examples: []string{"confluence-cli user get jdoe --compact"}},
		"confluence-cli user search":  {schema: "user_list", examples: []string{"confluence-cli user search 'John' --limit 10 --compact"}},

		// Task.
		"confluence-cli task get": {schema: "long_task", examples: []string{"confluence-cli task get 12345 --compact"}},

		// Page — core.
		"confluence-cli page get":         {schema: "page", examples: []string{"confluence-cli page get 12345 --body-format markdown --compact", `confluence-cli page get --space ENG --title "Roadmap" --compact`}},
		"confluence-cli page list":        {schema: "page_list", examples: []string{"confluence-cli page list --space ENG --limit 25 --compact"}},
		"confluence-cli page create":      {schema: "page", examples: []string{`confluence-cli page create --space ENG --title "Notes" --body "# Hi" --dry-run`}},
		"confluence-cli page update":      {schema: "page", examples: []string{`confluence-cli page update 12345 --title "Notes v2" --dry-run`}},
		"confluence-cli page delete":      {schema: "page_delete", examples: []string{"confluence-cli page delete 12345 --dangerous --dry-run"}},
		"confluence-cli page move":        {schema: "page", examples: []string{"confluence-cli page move 12345 --parent 67890 --dry-run"}},
		"confluence-cli page children":    {schema: "page_list", examples: []string{"confluence-cli page children 12345 --compact"}},
		"confluence-cli page descendants": {schema: "page_list", examples: []string{"confluence-cli page descendants 12345 --compact"}},
		"confluence-cli page ancestors":   {schema: "page_ancestors", examples: []string{"confluence-cli page ancestors 12345 --compact"}},
		"confluence-cli page history":     {schema: "page_history", examples: []string{"confluence-cli page history 12345 --compact"}},
		"confluence-cli page restore":     {schema: "page", examples: []string{"confluence-cli page restore 12345 --version 3 --dry-run"}},

		// Page — comments.
		"confluence-cli page comment list":   {schema: "comment_list", examples: []string{"confluence-cli page comment list 12345 --location all --compact"}},
		"confluence-cli page comment get":    {schema: "comment", examples: []string{"confluence-cli page comment get 99001 --compact"}},
		"confluence-cli page comment add":    {schema: "comment", examples: []string{`confluence-cli page comment add 12345 --body "LGTM" --dry-run`}},
		"confluence-cli page comment delete": {schema: "delete_result", examples: []string{"confluence-cli page comment delete 99001 --dangerous --dry-run"}},

		// Page — attachments.
		"confluence-cli page attachment list":     {schema: "attachment_list", examples: []string{"confluence-cli page attachment list 12345 --compact"}},
		"confluence-cli page attachment upload":   {schema: "attachment_upload", examples: []string{"confluence-cli page attachment upload 12345 --file ./diagram.png --dry-run"}},
		"confluence-cli page attachment download": {schema: "attachment_download", examples: []string{"confluence-cli page attachment download 55001 --out ./diagram.png"}},
		"confluence-cli page attachment delete":   {schema: "delete_result", examples: []string{"confluence-cli page attachment delete 55001 --dangerous --dry-run"}},

		// Page — labels.
		"confluence-cli page label list":   {schema: "label_list", examples: []string{"confluence-cli page label list 12345 --compact"}},
		"confluence-cli page label add":    {schema: "label_add", examples: []string{"confluence-cli page label add 12345 --labels adr,design --dry-run"}},
		"confluence-cli page label remove": {schema: "label_remove", examples: []string{"confluence-cli page label remove 12345 --labels adr,design --dry-run"}},
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
		"reference":     {Shape: "object", Fields: []string{"tool", "version", "schema_version", "risk_tier", "release_readiness", "root", "commands", "exit_codes", "error_codes", "schemas"}},
		"context":       {Shape: "object", Fields: []string{"tool", "version", "runtime", "config", "credentials", "account", "errors", "env"}},
		"doctor":        {Shape: "object", Fields: []string{"checks", "url", "username", "display_name", "server_version", "latency_ms"}},
		"changelog":     {Shape: "object", Fields: []string{"current_version", "since", "entries"}},
		"update_result": {Shape: "object", Fields: []string{"status", "current_version", "target_version", "requested_version", "previous_version", "knowledge_refresh", "update_available", "installed", "check_only", "dry_run", "install_method", "command", "asset", "path", "checksum_verified", "signature_status", "signature_verified", "skill_sync_command", "skill_sync_status", "notices"}},

		// Space (cmd/space.go).
		"space_list":   {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more"}, UntrustedFields: []string{"name", "description"}},
		"space":        {Shape: "object", Fields: []string{"key", "name", "type", "status", "description", "_untrusted"}, UntrustedFields: []string{"name", "description"}},
		"space_delete": {Shape: "object", Fields: []string{"task_id", "status", "status_link", "successful", "percentage_complete"}},

		// Search (cmd/search.go). Each result carries external title/excerpt.
		"search_result": {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more", "total_size"}, UntrustedFields: []string{"title", "excerpt"}},

		// User (cmd/user.go).
		"user":      {Shape: "object", Fields: []string{"username", "display_name", "user_key", "type", "_untrusted"}, UntrustedFields: []string{"display_name"}},
		"user_list": {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more", "total_size"}, UntrustedFields: []string{"display_name"}},

		// Task (cmd/task.go).
		"long_task": {Shape: "object", Fields: []string{"id", "name", "percentage_complete", "successful", "finished", "messages"}},

		// Page core (cmd/page.go). Title and body are external content.
		"page":           {Shape: "object", Fields: []string{"id", "title", "space_key", "status", "type", "version", "url", "body", "body_format", "body_fidelity", "unsupported_macros", "parent_id", "_untrusted"}, UntrustedFields: []string{"title", "body"}},
		"page_list":      {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more", "total_size"}, UntrustedFields: []string{"title"}},
		"page_delete":    {Shape: "object", Fields: []string{"id", "status", "purged"}},
		"page_ancestors": {Shape: "object", Fields: []string{"ancestors"}, UntrustedFields: []string{"title"}},
		"page_history":   {Shape: "object", Fields: []string{"id", "latest", "versions", "created_by", "created_date"}, UntrustedFields: []string{"created_by", "by", "message"}},

		// Page comments (cmd/page_comment.go).
		"comment":      {Shape: "object", Fields: []string{"id", "title", "body", "location", "resolution", "_untrusted"}, UntrustedFields: []string{"title", "body"}},
		"comment_list": {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more"}, UntrustedFields: []string{"title", "body"}},

		// Page attachments (cmd/page_attachment.go). Filenames are external.
		"attachment_list":     {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more"}, UntrustedFields: []string{"filename"}},
		"attachment_upload":   {Shape: "object", Fields: []string{"attachments", "count"}, UntrustedFields: []string{"filename"}},
		"attachment_download": {Shape: "object", Fields: []string{"path", "size_bytes"}},

		// Page labels (cmd/page_label.go).
		"label_list":   {Shape: "object", Fields: []string{"items", "count", "offset", "next_offset", "has_more"}, UntrustedFields: []string{"name"}},
		"label_add":    {Shape: "object", Fields: []string{"labels", "count"}, UntrustedFields: []string{"name"}},
		"label_remove": {Shape: "object", Fields: []string{"page_id", "ok", "items", "summary"}},

		// Shared delete result (comment/attachment delete).
		"delete_result": {Shape: "object", Fields: []string{"comment_id", "attachment_id", "status"}},
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
		string(output.ErrUsage):           "arguments or request are invalid",
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
