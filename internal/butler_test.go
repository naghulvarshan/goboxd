package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupRemovesStaleDirectories(t *testing.T) {
	home, _ := os.UserHomeDir()
	nsjailDir := home + "/nsjail_programs"
	if err := os.MkdirAll(nsjailDir, 0755); err != nil {
		t.Fatal(err)
	}

	staleDir := filepath.Join(nsjailDir, "nsip_stale_test_cleanup")
	if err := os.Mkdir(staleDir, 0755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(staleDir, past, past); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(staleDir) })

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}

	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Error("stale directory was not removed")
	}
}

func TestCleanupKeepsFreshDirectories(t *testing.T) {
	home, _ := os.UserHomeDir()
	nsjailDir := home + "/nsjail_programs"
	if err := os.MkdirAll(nsjailDir, 0755); err != nil {
		t.Fatal(err)
	}

	freshDir := filepath.Join(nsjailDir, "nsip_fresh_test_cleanup")
	if err := os.Mkdir(freshDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(freshDir) })

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}

	if _, err := os.Stat(freshDir); err != nil {
		t.Error("fresh directory was incorrectly removed")
	}
}

func TestCleanupMissingDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	original := home + "/nsjail_programs"

	// Only run this subtest if nsjail_programs doesn't exist (to avoid disrupting real state)
	if _, err := os.Stat(original); os.IsNotExist(err) {
		err := cleanup()
		if err == nil {
			t.Error("expected error when nsjail_programs dir is missing, got nil")
		}
	} else {
		t.Skip("nsjail_programs already exists; skipping missing-dir test")
	}
}
