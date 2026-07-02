package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func overrideHome(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	origURL := os.Getenv(EnvURL)
	origToken := os.Getenv(EnvToken)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("USERPROFILE", tmpDir)
	_ = os.Unsetenv(EnvURL)
	_ = os.Unsetenv(EnvToken)
	return tmpDir, func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("USERPROFILE", origUserProfile)
		_ = os.Setenv(EnvURL, origURL)
		_ = os.Setenv(EnvToken, origToken)
	}
}

func TestSaveAndLoad(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	want := &Config{
		URL:   "https://confluence.example.com",
		Token: "pat-secret-token",
	}

	if err := Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if got.URL != want.URL || got.Token != want.Token {
		t.Errorf("got %+v, want %+v", got, want)
	}

	data, err := os.ReadFile(FilePath())
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if strings.Contains(string(data), want.Token) {
		t.Fatalf("config file contains plaintext token: %s", data)
	}
	if !strings.Contains(string(data), "token_storage") {
		t.Fatalf("config file should declare its storage backend: %s", data)
	}
}

func TestLoadLegacyPlaintextConfig(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	writeConfigFile(t, `{"url":"https://confluence.example.com","token":"legacy-token"}`)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.URL != "https://confluence.example.com" || got.Token != "legacy-token" {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	tmpDir, restore := overrideHome(t)
	defer restore()

	cfg := &Config{URL: "https://confluence.example.com", Token: "tok"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	info, err := os.Stat(filepath.Join(tmpDir, ".confluence-cli"))
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}

func writeConfigFile(t *testing.T, content string) {
	t.Helper()
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(FilePath(), []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	writeConfigFile(t, `{not valid json`)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing config") {
		t.Errorf("error = %q, want parsing config message", err.Error())
	}
}

func TestMustLoadInvalidJSON(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	writeConfigFile(t, `{not valid json`)

	_, err := MustLoad()
	if err == nil {
		t.Fatal("MustLoad() should return error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing config") {
		t.Errorf("error = %q, want parsing config message", err.Error())
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not error when file missing, got: %v", err)
	}
	if cfg.URL != "" || cfg.Token != "" {
		t.Errorf("expected empty Config, got %+v", cfg)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	// Save a config file
	want := &Config{
		URL:   "https://file-host.example.com",
		Token: "file-token",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Set env vars that should override
	t.Setenv(EnvURL, "https://env-host.example.com")
	t.Setenv(EnvToken, "env-token")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.URL != "https://env-host.example.com" {
		t.Errorf("URL = %q, want env override", got.URL)
	}
	if got.Token != "env-token" {
		t.Errorf("Token = %q, want env override", got.Token)
	}
}

func TestLoadEnvPartialOverride(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	// Save a config file
	if err := Save(&Config{URL: "https://file-host.example.com", Token: "file-token"}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Only override token via env
	t.Setenv(EnvToken, "env-token-only")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.URL != "https://file-host.example.com" {
		t.Errorf("URL = %q, want file value", got.URL)
	}
	if got.Token != "env-token-only" {
		t.Errorf("Token = %q, want env override", got.Token)
	}
}

func TestMustLoadValid(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	want := &Config{
		URL:   "https://confluence.example.com",
		Token: "pat-secret",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := MustLoad()
	if err != nil {
		t.Fatalf("MustLoad() unexpected error: %v", err)
	}
	if got.URL != want.URL || got.Token != want.Token {
		t.Errorf("MustLoad() got %+v, want %+v", got, want)
	}
}

func TestMustLoadMissingFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty", Config{}},
		{"missing url", Config{Token: "tok"}},
		{"missing token", Config{URL: "https://confluence.example.com"}},
		{"url not https", Config{URL: "http://confluence.example.com", Token: "tok"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, restore := overrideHome(t)
			defer restore()

			if err := Save(&tc.cfg); err != nil {
				t.Fatalf("Save() error: %v", err)
			}

			_, err := MustLoad()
			if err == nil {
				t.Error("MustLoad() should return error when fields are missing")
			}
		})
	}
}

func TestMustLoadEnvOverride(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	// No config file, but env vars set
	t.Setenv(EnvURL, "https://env.example.com")
	t.Setenv(EnvToken, "env-token")

	got, err := MustLoad()
	if err != nil {
		t.Fatalf("MustLoad() unexpected error: %v", err)
	}
	if got.URL != "https://env.example.com" || got.Token != "env-token" {
		t.Errorf("MustLoad() got %+v", got)
	}
}

func TestDeleteThenLoad(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	cfg := &Config{URL: "https://confluence.example.com", Token: "tok"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if err := Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() after Delete() error: %v", err)
	}
	if got.URL != "" || got.Token != "" {
		t.Errorf("expected empty Config after Delete, got %+v", got)
	}
}

func TestDeleteIdempotent(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	if err := Delete(); err != nil {
		t.Errorf("Delete() on non-existent file should not error, got: %v", err)
	}
}

func TestIsConfigured(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	if IsConfigured() {
		t.Error("IsConfigured() should be false when no config exists")
	}

	if err := Save(&Config{URL: "https://confluence.example.com", Token: "tok"}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if !IsConfigured() {
		t.Error("IsConfigured() should be true after saving valid config")
	}
}

func TestIsConfiguredEnvVars(t *testing.T) {
	_, restore := overrideHome(t)
	defer restore()

	if IsConfigured() {
		t.Error("IsConfigured() should be false initially")
	}

	t.Setenv(EnvURL, "https://confluence.example.com")
	t.Setenv(EnvToken, "tok")

	if !IsConfigured() {
		t.Error("IsConfigured() should be true with env vars set")
	}
}
