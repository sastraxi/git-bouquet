package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadValid(t *testing.T) {
	dir := writeConfig(t, `target: release/current
base: main
merge:
  - feat/*
  - test/*
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Target != "release/current" || c.Base != "main" {
		t.Errorf("target/base: %+v", c)
	}
	if len(c.Merge) != 2 {
		t.Errorf("merge len: %d", len(c.Merge))
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no .bouquet.yaml") {
		t.Errorf("expected missing-file error, got %v", err)
	}
}

func TestLoadUnknownField(t *testing.T) {
	dir := writeConfig(t, `target: t
base: b
merge: [x]
bogus: 1
`)
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		c       Config
		wantErr string
	}{
		{"missing target", Config{Base: "b", Merge: []string{"x"}}, "`target` is required"},
		{"missing base", Config{Target: "t", Merge: []string{"x"}}, "`base` is required"},
		{"target==base", Config{Target: "x", Base: "x", Merge: []string{"y"}}, "must differ"},
		{"empty merge", Config{Target: "t", Base: "b"}, "`merge` must contain"},
		{"empty merge entry", Config{Target: "t", Base: "b", Merge: []string{"x", "  "}}, "is empty"},
		{"valid", Config{Target: "t", Base: "b", Merge: []string{"x"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}
