package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type marshalFail struct{}

func (marshalFail) MarshalJSON() ([]byte, error) {
	return nil, errors.New("marshal failed")
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	fn()

	_ = w.Close()
	os.Stderr = orig

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestErrorCodeFromStatus(t *testing.T) {
	cases := []struct {
		code int
		want ErrorCode
	}{
		{401, ErrAuth},
		{403, ErrForbidden},
		{404, ErrNotFound},
		{408, ErrTimeout},
		{409, ErrConflict},
		{429, ErrRateLimit},
		{500, ErrServer},
		{503, ErrServer},
		{400, ErrValidation},
		{422, ErrValidation},
		{200, ErrUnknown},
		{0, ErrUnknown},
	}
	for _, tc := range cases {
		if got := ErrorCodeFromStatus(tc.code); got != tc.want {
			t.Errorf("ErrorCodeFromStatus(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestHintForErrorCode(t *testing.T) {
	// Every known code must have a hint except ErrUnknown; unknown codes get "".
	for _, code := range allErrorCodes {
		hint := HintForErrorCode(code)
		if code == ErrUnknown {
			if hint != "" {
				t.Errorf("HintForErrorCode(ErrUnknown) = %q, want empty", hint)
			}
			continue
		}
		if hint == "" {
			t.Errorf("HintForErrorCode(%q) should be non-empty", code)
		}
	}
	if HintForErrorCode(ErrorCode("OTHER")) != "" {
		t.Error("unknown code should have empty hint")
	}
	if !strings.Contains(HintForErrorCode(ErrConfig), "CONFLUENCE_CLI_URL") {
		t.Error("E_CONFIG hint should mention CONFLUENCE_CLI_URL env var")
	}
}

func TestPrintJSON_MarshalError(t *testing.T) {
	out := captureStdout(t, func() {
		PrintJSON(marshalFail{})
	})
	if !strings.Contains(out, `"ok": false`) || !strings.Contains(out, "marshal failed") {
		t.Errorf("PrintJSON marshal error: got %q", out)
	}
}

func TestPrintJSON_Compact(t *testing.T) {
	old := CompactJSON
	CompactJSON = true
	t.Cleanup(func() { CompactJSON = old })

	out := captureStdout(t, func() {
		PrintJSON(map[string]string{"hello": "world"})
	})
	line := strings.TrimSpace(out)
	if strings.Contains(line, "\n") || strings.Contains(line, "  ") {
		t.Errorf("compact output should have no indentation, got %q", line)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("compact output not valid JSON: %v", err)
	}
}

func TestPrintJSON_RawFormatSkipsEnvelope(t *testing.T) {
	old := EnvelopeJSON
	EnvelopeJSON = false
	t.Cleanup(func() { EnvelopeJSON = old })

	out := captureStdout(t, func() {
		PrintJSON(map[string]string{"hello": "world"})
	})
	if strings.Contains(out, "schema_version") {
		t.Errorf("EnvelopeJSON=false should not wrap payload, got %q", out)
	}
}

func TestPrintJSON_MetaNotices(t *testing.T) {
	t.Run("present when provider has notices", func(t *testing.T) {
		old := UpdateNoticesProvider
		t.Cleanup(func() { UpdateNoticesProvider = old })
		UpdateNoticesProvider = func() []any {
			return []any{map[string]any{"type": "update_available", "severity": "warning"}}
		}
		out := captureStdout(t, func() { PrintJSON(map[string]any{"x": 1}) })
		var payload struct {
			Meta struct {
				DurationMS int64            `json:"duration_ms"`
				Notices    []map[string]any `json:"notices"`
			} `json:"meta"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
			t.Fatalf("invalid JSON: %v\nout: %s", err, out)
		}
		if len(payload.Meta.Notices) != 1 {
			t.Fatalf("meta.notices = %v, want 1 notice", payload.Meta.Notices)
		}
		if payload.Meta.Notices[0]["severity"] != "warning" {
			t.Errorf("severity = %v, want warning", payload.Meta.Notices[0]["severity"])
		}
	})

	t.Run("absent when provider empty", func(t *testing.T) {
		old := UpdateNoticesProvider
		t.Cleanup(func() { UpdateNoticesProvider = old })
		UpdateNoticesProvider = func() []any { return nil }
		out := captureStdout(t, func() { PrintJSON(map[string]any{"x": 1}) })
		if strings.Contains(out, "notices") {
			t.Errorf("meta.notices should be omitted, got %q", out)
		}
		if !strings.Contains(out, "duration_ms") {
			t.Errorf("meta.duration_ms must always be present, got %q", out)
		}
	})

	t.Run("absent when provider nil", func(t *testing.T) {
		old := UpdateNoticesProvider
		t.Cleanup(func() { UpdateNoticesProvider = old })
		UpdateNoticesProvider = nil
		out := captureStdout(t, func() { PrintJSON(map[string]any{"x": 1}) })
		if strings.Contains(out, "notices") {
			t.Errorf("meta.notices should be omitted, got %q", out)
		}
	})
}

func TestPrintErrorJSON(t *testing.T) {
	t.Run("with status", func(t *testing.T) {
		out := captureStdout(t, func() {
			PrintErrorJSON("not found", 404)
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
			t.Fatalf("invalid JSON: %v\nout: %s", err, out)
		}
		errPayload := payload["error"].(map[string]any)
		if errPayload["message"] != "not found" {
			t.Errorf("message = %v", errPayload["message"])
		}
		if errPayload["code"] != string(ErrNotFound) {
			t.Errorf("code = %v, want %s", errPayload["code"], ErrNotFound)
		}
		details := errPayload["details"].(map[string]any)
		if details["hint"] == "" {
			t.Error("expected non-empty hint for 404")
		}
		if details["status_code"] != float64(404) {
			t.Errorf("status_code = %v, want 404", details["status_code"])
		}
	})

	t.Run("status zero uses unknown", func(t *testing.T) {
		out := captureStdout(t, func() {
			PrintErrorJSON("oops", 0)
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		errPayload := payload["error"].(map[string]any)
		if errPayload["code"] != string(ErrUnknown) {
			t.Errorf("code = %v, want %s", errPayload["code"], ErrUnknown)
		}
	})
}

func TestPrintErrorJSONWithCode(t *testing.T) {
	out := captureStdout(t, func() {
		PrintErrorJSONWithCode("network down", 0, ErrNetwork)
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\nout: %s", err, out)
	}
	errPayload := payload["error"].(map[string]any)
	if errPayload["code"] != string(ErrNetwork) {
		t.Errorf("code = %v, want %s", errPayload["code"], ErrNetwork)
	}
	if errPayload["retryable"] != true {
		t.Errorf("retryable = %v", errPayload["retryable"])
	}
	details := errPayload["details"].(map[string]any)
	if details["hint"] != HintForErrorCode(ErrNetwork) {
		t.Errorf("hint = %v", details["hint"])
	}
}

func TestPrintErrorJSONWithDetails(t *testing.T) {
	out := captureStdout(t, func() {
		PrintErrorJSONWithDetails("update failed", 0, ErrIO, map[string]any{
			"stage":           "replace",
			"binary_replaced": false,
		})
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\nout: %s", err, out)
	}
	errPayload := payload["error"].(map[string]any)
	details := errPayload["details"].(map[string]any)
	if details["stage"] != "replace" {
		t.Errorf("details.stage = %v, want replace", details["stage"])
	}
	if details["binary_replaced"] != false {
		t.Errorf("details.binary_replaced = %v, want false", details["binary_replaced"])
	}
}

func TestPrintErrorJSON_MarshalFallback(t *testing.T) {
	old := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("indent failed")
	}
	t.Cleanup(func() { jsonMarshalIndent = old })

	out := captureStdout(t, func() {
		PrintErrorJSON("boom", 500)
	})
	if !strings.Contains(out, `"message":"boom"`) || !strings.Contains(out, string(ErrServer)) {
		t.Errorf("PrintErrorJSON fallback: got %q", out)
	}
}

func TestPrintErrorJSONWithCode_MarshalFallback(t *testing.T) {
	old := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("indent failed")
	}
	t.Cleanup(func() { jsonMarshalIndent = old })

	out := captureStdout(t, func() {
		PrintErrorJSONWithCode("bad", 0, ErrConfig)
	})
	if !strings.Contains(out, `"message":"bad"`) || !strings.Contains(out, string(ErrConfig)) {
		t.Errorf("PrintErrorJSONWithCode fallback: got %q", out)
	}
}
