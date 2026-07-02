package main

import (
	"os"
	"testing"

	"github.com/fatecannotbealtered/confluence-cli/cmd"
)

func TestMain_ReturnsZero(t *testing.T) {
	origArgs := os.Args
	os.Args = []string{"confluence-cli", "reference"}
	t.Cleanup(func() { os.Args = origArgs })

	if code := run(); code != cmd.ExitOK {
		t.Fatalf("run() = %d, want 0", code)
	}
}

func TestMain_ExitFn(t *testing.T) {
	origExit := exitFn
	origArgs := os.Args
	t.Cleanup(func() {
		exitFn = origExit
		os.Args = origArgs
	})
	os.Args = []string{"confluence-cli", "reference"}
	var gotCode int
	exitFn = func(code int) { gotCode = code }
	main()
	if gotCode != cmd.ExitOK {
		t.Fatalf("main exit = %d, want 0", gotCode)
	}
}

func TestMain_SilentErrorViaDoctor(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("CONFLUENCE_CLI_URL", "")
	t.Setenv("CONFLUENCE_CLI_TOKEN", "")
	t.Setenv("CONFLUENCE_CLI_NO_AUDIT", "1")

	origArgs := os.Args
	os.Args = []string{"confluence-cli", "doctor"}
	t.Cleanup(func() { os.Args = origArgs })

	if code := run(); code != cmd.ExitAuth {
		t.Fatalf("run() = %d, want %d", code, cmd.ExitAuth)
	}
}
