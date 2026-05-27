package config

import (
	"fmt"
	"os"

	"github.com/thesouldev/goboxd/internal/types"

	"github.com/ghodss/yaml"
)

// LoadConfig reads the path provided and loads the config file into types.Config struct
func LoadConfig(path string) (*types.Config, error) {
	out, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}
	var cfg *types.Config
	if err = yaml.Unmarshal(out, &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling config file: %v", err)
	}
	return cfg, nil
}
