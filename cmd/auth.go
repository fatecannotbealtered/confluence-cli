package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/config"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Test seams for the interactive token prompt.
var (
	loginIsTerminal   = term.IsTerminal
	loginReadPassword = term.ReadPassword
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Confluence Data Center credentials",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Configure Confluence Data Center credentials",
	Long: `Configure Confluence Data Center base URL and Personal Access Token (PAT).

  Interactive:     confluence-cli auth login (with --format text)
  Non-interactive: confluence-cli auth login --url https://confluence.company.com --token <PAT>

  Credentials are validated against GET /rest/api/user/current before being
  saved (OS keyring, with machine-bound encrypted file as fallback).
  Environment variables CONFLUENCE_CLI_URL and CONFLUENCE_CLI_TOKEN override
  the saved config.`,
	Args: cobra.NoArgs,
	RunE: runAuthLogin,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove saved Confluence credentials",
	Args:  cobra.NoArgs,
	RunE:  runAuthLogout,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show credential status (redacted): URL, token fingerprint, username",
	Args:  cobra.NoArgs,
	RunE:  runAuthStatus,
}

var (
	loginURLFlag   string
	loginTokenFlag string
)

func init() {
	authLoginCmd.Flags().StringVar(&loginURLFlag, "url", "", "Confluence base URL (e.g. https://confluence.company.com)")
	authLoginCmd.Flags().StringVar(&loginTokenFlag, "token", "", "Personal Access Token (PAT)")
	markWrite(authLoginCmd)
	markWrite(authLogoutCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthLogin(_ *cobra.Command, _ []string) error {
	// Non-interactive mode: both --url and --token provided.
	if loginURLFlag != "" && loginTokenFlag != "" {
		u := strings.TrimSpace(loginURLFlag)
		if !strings.HasPrefix(u, "https://") {
			output.Error("url must start with https://")
			return SilentErr(ExitBadArgs)
		}
		token := strings.TrimSpace(loginTokenFlag)
		if token == "" {
			output.Error("token cannot be empty")
			return SilentErr(ExitBadArgs)
		}
		loginDetail := map[string]any{"url": u, "token_sha256": tokenFingerprint(token)}
		if dryRunOutput("auth login", loginDetail) {
			return nil
		}
		return finishLogin(u, token)
	}

	// Interactive mode (text format only).
	if jsonMode {
		output.Error("auth login requires --url and --token in json format; use --format text for interactive login")
		return SilentErr(ExitBadArgs)
	}
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	output.Bold("  confluence-cli Login (Data Center)")
	output.Gray("  ────────────────────────────────────────")
	fmt.Println()

	u := loginURLFlag
	if u == "" {
		fmt.Print("  Confluence base URL (e.g. https://confluence.company.com): ")
		u, _ = reader.ReadString('\n')
		u = strings.TrimSpace(u)
	}
	if !strings.HasPrefix(u, "https://") {
		output.Error("url must start with https://")
		return SilentErr(ExitBadArgs)
	}

	token := loginTokenFlag
	if token == "" {
		fmt.Print("  Personal Access Token (PAT): ")
		var tokenBytes []byte
		var err error
		if loginIsTerminal(int(syscall.Stdin)) {
			tokenBytes, err = loginReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil {
				output.Error("failed to read token: " + err.Error())
				return SilentErr(ExitBadArgs)
			}
		} else {
			line, _ := reader.ReadString('\n')
			tokenBytes = []byte(strings.TrimSpace(line))
		}
		token = strings.TrimSpace(string(tokenBytes))
	}
	if token == "" {
		output.Error("token cannot be empty")
		return SilentErr(ExitBadArgs)
	}

	output.Gray("  Verifying credentials...")
	return finishLogin(u, token)
}

// finishLogin validates the credentials against GET /rest/api/user/current and
// only persists them (keyring / encrypted file) after the probe succeeds.
func finishLogin(u, token string) error {
	client := api.NewClient(u, token, api.Options{Version: version})
	me, err := client.Users.CurrentUser()
	if err != nil {
		output.Error("invalid credentials: " + err.Error())
		return SilentErr(ExitAuth)
	}

	if err := config.Save(&config.Config{URL: u, Token: token}); err != nil {
		output.Error("failed to save config: " + err.Error())
		return SilentErr(ExitAuth)
	}

	if jsonMode {
		output.PrintJSON(map[string]string{
			"status":       "ok",
			"display_name": me.DisplayName,
			"username":     me.Username,
		})
		return nil
	}
	fmt.Println()
	output.Success(fmt.Sprintf("Logged in as %s (%s)", me.DisplayName, me.Username))
	output.Info("Config saved to " + config.FilePath())
	fmt.Println()
	output.Gray("  Try: confluence-cli doctor")
	fmt.Println()
	return nil
}

func runAuthLogout(_ *cobra.Command, _ []string) error {
	if dryRunOutput("auth logout", map[string]any{"config_file": config.FilePath()}) {
		return nil
	}
	if err := config.Delete(); err != nil {
		output.Error("failed to remove config: " + err.Error())
		return SilentErr(ExitAuth)
	}
	if jsonMode {
		output.PrintJSON(map[string]string{"status": "loggedOut"})
		return nil
	}
	output.Success("Logged out. Config removed.")
	return nil
}

type authStatusDocument struct {
	Status       string `json:"status"`
	URL          string `json:"url,omitempty"`
	TokenPresent bool   `json:"token_present"`
	TokenSHA256  string `json:"token_sha256,omitempty"`
	Storage      string `json:"storage,omitempty"`
	Username     string `json:"username,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Error        string `json:"error,omitempty"`
}

func runAuthStatus(_ *cobra.Command, _ []string) error {
	doc := authStatusDocument{Status: "not_configured"}

	cfg, err := config.Load()
	if err != nil {
		doc.Status = "config_error"
		doc.Error = err.Error()
		printAuthStatus(doc)
		return nil
	}
	doc.URL = cfg.URL
	doc.TokenPresent = strings.TrimSpace(cfg.Token) != ""
	doc.Storage = cfg.Storage
	if doc.TokenPresent {
		doc.TokenSHA256 = tokenFingerprint(strings.TrimSpace(cfg.Token))
	}
	if cfg.URL == "" || !doc.TokenPresent {
		printAuthStatus(doc)
		return nil
	}

	me, err := api.NewClient(cfg.URL, cfg.Token, api.Options{Version: version}).Users.CurrentUser()
	if err != nil {
		doc.Status = "invalid"
		doc.Error = err.Error()
		printAuthStatus(doc)
		return nil
	}
	doc.Status = "valid"
	doc.Username = me.Username
	doc.DisplayName = me.DisplayName
	printAuthStatus(doc)
	return nil
}

func printAuthStatus(doc authStatusDocument) {
	if jsonMode {
		output.PrintJSON(doc)
		return
	}
	output.Bold("confluence-cli auth status")
	if doc.URL != "" {
		output.Gray("  URL: " + doc.URL)
	}
	if doc.TokenSHA256 != "" {
		output.Gray("  Token: sha256:" + doc.TokenSHA256)
	}
	output.Gray("  Status: " + doc.Status)
	if doc.Username != "" {
		output.Gray(fmt.Sprintf("  Account: %s (%s)", doc.DisplayName, doc.Username))
	}
	if doc.Error != "" {
		output.Gray("  Error: " + doc.Error)
	}
}

// tokenFingerprint returns a short, non-reversible identifier for a token.
func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}
