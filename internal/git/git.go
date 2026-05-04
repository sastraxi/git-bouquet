// Package git is a thin shell-out wrapper around the user's installed git
// binary. We deliberately avoid go-git so that user config (rerere, hooks,
// signing, gitattributes) applies exactly as it would at the command line.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a handle to a git repository, optionally rooted in a specific
// working directory (used for the bouquet worktree).
type Repo struct {
	// Dir is the working directory commands run in. Empty means the current
	// process working directory.
	Dir string
}

// Root returns a Repo rooted at the current working directory.
func Root() *Repo { return &Repo{} }

// In returns a Repo rooted at dir.
func (r *Repo) In(dir string) *Repo { return &Repo{Dir: dir} }

func (r *Repo) cmd(args ...string) *exec.Cmd {
	c := exec.Command("git", args...)
	if r.Dir != "" {
		c.Dir = r.Dir
	}
	return c
}

// Run runs git with the given args, streaming stdout/stderr to the parent.
func (r *Repo) Run(args ...string) error {
	c := r.cmd(args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// Quiet runs git with stdout/stderr suppressed; returns the combined error.
func (r *Repo) Quiet(args ...string) error {
	c := r.cmd(args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// Output runs git and returns trimmed stdout. Stderr is captured into the
// returned error on failure.
func (r *Repo) Output(args ...string) (string, error) {
	c := r.cmd(args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// ExitCode runs git and returns its exit code with stdio streamed.
// Useful for predicates like merge-base --is-ancestor where 0/1 are expected.
func (r *Repo) ExitCode(args ...string) (int, error) {
	c := r.cmd(args...)
	c.Stdout = nil
	c.Stderr = nil
	err := c.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, err
}

// GitDir returns the absolute path to the .git directory (or git common dir
// for worktrees). Always run against the parent repo, not a worktree.
func (r *Repo) GitDir() (string, error) {
	out, err := r.Output("rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(out) {
		return out, nil
	}
	abs, err := filepath.Abs(filepath.Join(r.Dir, out))
	if err != nil {
		return "", err
	}
	return abs, nil
}

// TopLevel returns the absolute path to the repo working tree root.
func (r *Repo) TopLevel() (string, error) {
	return r.Output("rev-parse", "--show-toplevel")
}

// LocalBranches returns all local branch short names.
func (r *Repo) LocalBranches() ([]string, error) {
	out, err := r.Output("for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// RevParse returns the SHA for a ref. Empty string + nil error if it doesn't
// exist (target branch on first run).
func (r *Repo) RevParse(ref string) (string, error) {
	code, err := r.ExitCode("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return r.Output("rev-parse", ref+"^{commit}")
}

// IsAncestor reports whether a is an ancestor commit of b.
func (r *Repo) IsAncestor(a, b string) (bool, error) {
	code, err := r.ExitCode("merge-base", "--is-ancestor", a, b)
	if err != nil {
		return false, err
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("merge-base --is-ancestor exited %d", code)
	}
}

// HasUnmergedPaths reports whether the current index has conflict markers.
func (r *Repo) HasUnmergedPaths() (bool, error) {
	out, err := r.Output("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// ModifyDeletePaths returns paths with a modify/delete conflict in either
// direction: "DU" (we deleted, they modified) or "UD" (they deleted, we
// modified). Both cases resolve identically with `git rm`. Rerere cannot
// cache either.
func (r *Repo) ModifyDeletePaths() ([]string, error) {
	out, err := r.Output("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "DU ") || strings.HasPrefix(line, "UD ") {
			paths = append(paths, line[3:])
		}
	}
	return paths, nil
}

// MergeInProgress reports whether MERGE_HEAD exists in this repo/worktree.
func (r *Repo) MergeInProgress() (bool, error) {
	gitDir, err := r.Output("rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(r.Dir, gitDir)
	}
	_, err = os.Stat(filepath.Join(gitDir, "MERGE_HEAD"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ConfigGet returns a git config value, or empty string if unset.
func (r *Repo) ConfigGet(key string) (string, error) {
	code, err := r.ExitCode("config", "--get", key)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return r.Output("config", "--get", key)
}

// ConfigSet sets a git config value at the local (repo) scope.
func (r *Repo) ConfigSet(key, value string) error {
	return r.Quiet("config", key, value)
}
