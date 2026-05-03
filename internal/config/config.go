// Package config loads and validates .bouquet.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const Filename = ".bouquet.yaml"

type Config struct {
	Target string   `yaml:"target"`
	Base   string   `yaml:"base"`
	Merge  []string `yaml:"merge"`
}

// Load reads and validates the config file at <repoRoot>/.bouquet.yaml.
func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, Filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s found at repo root (%s)", Filename, repoRoot)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Target) == "" {
		return fmt.Errorf("`target` is required")
	}
	if strings.TrimSpace(c.Base) == "" {
		return fmt.Errorf("`base` is required")
	}
	if c.Target == c.Base {
		return fmt.Errorf("`target` and `base` must differ")
	}
	if len(c.Merge) == 0 {
		return fmt.Errorf("`merge` must contain at least one glob")
	}
	for i, p := range c.Merge {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("`merge[%d]` is empty", i)
		}
	}
	return nil
}
