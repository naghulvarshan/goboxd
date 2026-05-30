package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/ghodss/yaml"
	"github.com/thesouldev/goboxd/internal/config"
	server "github.com/thesouldev/goboxd/internal/server"
)

var cfgPath = "/usr/local/bin/config.yaml"

func main() {
	var port string
	// TODO: Replace with ENV based log level
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	flag.StringVar(&port, "port", "8080", "--port 8080")
	flag.Parse()

	cf, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	out, _ := yaml.Marshal(cf)
	if err := InitCgroupBase(); err != nil {
		slog.Warn("cgroup memory tracking disabled", "error", err)
	}

	home, _ := os.UserHomeDir()
	os.Mkdir(home+"/nsjail_programs", 0755)
	slog.Debug("config", "config content", string(out))
	server.Serve(port, cf)
}

func InitCgroupBase() error {
	// cgroup.controllers only exists on cgroupv2 mounts
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		return fmt.Errorf("cgroupv2 not available at /sys/fs/cgroup: %w", err)
	}
	const base = "/sys/fs/cgroup/goboxd"
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	return os.WriteFile(base+"/cgroup.subtree_control", []byte("+memory"), 0644)
}
