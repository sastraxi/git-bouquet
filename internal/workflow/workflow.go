// Package workflow implements the bouquet subcommands.
package workflow

import (
	"fmt"
	"os"

	"github.com/sastraxi/git-bouquet/internal/config"
	"github.com/sastraxi/git-bouquet/internal/git"
	"github.com/sastraxi/git-bouquet/internal/state"
)

// Env bundles the per-invocation context used by every subcommand.
type Env struct {
	Repo     *git.Repo
	RepoRoot string
	GitDir   string
	Paths    state.Paths
	Config   *config.Config
}

// Setup loads config + locates state paths. Used by every subcommand.
func Setup() (*Env, error) {
	r := git.Root()
	root, err := r.TopLevel()
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository (%w)", err)
	}
	gitDir, err := r.GitDir()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	return &Env{
		Repo:     r,
		RepoRoot: root,
		GitDir:   gitDir,
		Paths:    state.Locate(gitDir),
		Config:   cfg,
	}, nil
}

func info(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
