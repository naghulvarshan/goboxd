package main

import (
	"flag"
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
	slog.Debug("config", "config content", string(out))
	server.Serve(port, cf)
}
