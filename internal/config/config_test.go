package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := writeConfig(t, `base: main
branches:
  release/current:
    - feat/*
    - fix/*
`)
	defer os.RemoveAll(dir)

	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if c.Base != "main" {
		t.Errorf("base: %s", c.Base)
	}
	if len(c.Branches) != 1 {
		t.Errorf("branches len: %d", len(c.Branches))
	}
	merge := c.Branches["release/current"]
	if !reflect.DeepEqual(merge, []string{"feat/*", "fix/*"}) {
		t.Errorf("merge: %v", merge)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{"missing base", Config{Branches: map[string][]string{"t": {"x"}}}, "`base` is required"},
		{"missing branches", Config{Base: "b"}, "`branches` must contain"},
		{"empty branch name", Config{Base: "b", Branches: map[string][]string{"": {"x"}}}, "branch target name cannot be empty"},
		{"target==base", Config{Base: "x", Branches: map[string][]string{"x": {"y"}}}, "cannot be the same as base"},
		{"empty merge", Config{Base: "b", Branches: map[string][]string{"t": {}}}, "must contain at least one merge glob"},
		{"empty merge entry", Config{Base: "b", Branches: map[string][]string{"t": {"x", "  "}}}, "is empty"},
		{"valid", Config{Base: "b", Branches: map[string][]string{"t": {"x"}}}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error %q, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "bouquet-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
