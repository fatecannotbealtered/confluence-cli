package cmd

import (
	"errors"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/config"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
)

// Clock/poll seams for long-task polling (overridden in tests to avoid real waits).
var (
	clockNow     = time.Now
	clockSleep   = time.Sleep
	pollInterval = 2 * time.Second
)

// newClient loads credentials and constructs an API client. On a missing or
// invalid configuration it emits the E_CONFIG envelope and returns a non-nil
// error (already reported) so the caller can return SilentErr.
func newClient() (*api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		emitError(output.ErrConfig, err.Error(), nil)
		return nil, SilentErr(ExitAuth)
	}
	if cfg.URL == "" || cfg.Token == "" {
		emitError(output.ErrConfig, "Confluence URL/token not configured; run 'confluence-cli auth login' or set "+config.EnvURL+" and "+config.EnvToken, nil)
		return nil, SilentErr(ExitAuth)
	}
	return api.NewClient(cfg.URL, cfg.Token, api.Options{Version: version}), nil
}

// emitError writes an error envelope (JSON) or a plain message (text) with the
// given code and optional structured details.
func emitError(code output.ErrorCode, msg string, details map[string]any) {
	if jsonMode {
		output.PrintErrorJSONWithDetails(msg, 0, code, details)
	} else {
		output.Error(msg)
	}
}

// emitAPIError maps an *api.APIError (or any error) onto the canonical envelope
// and returns SilentErr with the matching exit code. The upstream E_* code and
// structured details (server_message, http_status, ...) are preserved so an
// agent sees the same taxonomy the client produced.
func emitAPIError(err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		code := output.ErrorCode(apiErr.Code)
		emitError(code, apiErr.Message, apiErr.Details)
		return SilentErr(output.ExitCodeForErrorCode(code))
	}
	emitError(output.ErrUnknown, err.Error(), nil)
	return SilentErr(ExitGeneric)
}
