package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		f := writeTempFile(t, `
nsjail_path: "/usr/local/bin/nsjail"
default_common_settings:
  nsjail_args: "-B /bin"
languages:
  - id: py3
    source: test.py
    run:
      cmd: /usr/bin/python3
      args: ["{{source}}"]
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 64
    version_cmd: "/usr/bin/python3 --version"
`)
		cfg, err := LoadConfig(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.NsjailPath != "/usr/local/bin/nsjail" {
			t.Errorf("NsjailPath = %q, want %q", cfg.NsjailPath, "/usr/local/bin/nsjail")
		}
		if len(cfg.LanguageSettings) != 1 {
			t.Fatalf("len(LanguageSettings) = %d, want 1", len(cfg.LanguageSettings))
		}
		if cfg.LanguageSettings[0].Id != "py3" {
			t.Errorf("language id = %q, want %q", cfg.LanguageSettings[0].Id, "py3")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path/config.yaml")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		f := writeTempFile(t, `{bad yaml: [}`)
		_, err := LoadConfig(f)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty file does not error", func(t *testing.T) {
		f := writeTempFile(t, ``)
		_, err := LoadConfig(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return f
}
