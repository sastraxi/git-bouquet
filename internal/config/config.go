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
	Base     string              `yaml:"base"`
	Branches map[string][]string `yaml:"branches"`
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
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Base) == "" {
		return fmt.Errorf("`base` is required")
	}
	if len(c.Branches) == 0 {
		return fmt.Errorf("`branches` must contain at least one target")
	}
	for target, merge := range c.Branches {
		if strings.TrimSpace(target) == "" {
			return fmt.Errorf("branch target name cannot be empty")
		}
		if target == c.Base {
			return fmt.Errorf("branch target %q cannot be the same as base %q", target, c.Base)
		}
		if len(merge) == 0 {
			return fmt.Errorf("branch %q must contain at least one merge glob", target)
		}
		for i, p := range merge {
			if strings.TrimSpace(p) == "" {
				return fmt.Errorf("branch %q: merge[%d] is empty", target, i)
			}
		}
	}
	return nil
}
