package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func junkCleaner(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanup()
		case <-ctx.Done():
			return
		}
	}
}

func cleanup() error {
	slog.Info("clearing stale driectories")
	home, _ := os.UserHomeDir()
	dir := home + "/nsjail_programs"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-1 * time.Minute)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(dir, e.Name()))
			slog.Info("freeing workspace", "workspace", e.Name())
		}
	}
	return nil
}
