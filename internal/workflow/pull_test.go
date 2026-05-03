package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sastraxi/git-bouquet/internal/git"
)

// setupRepoWithRemote builds a tmp git repo wired to a bare "remote" repo,
// with `branch` tracking origin/branch. Returns (repoPath, remotePath).
func setupRepoWithRemote(t *testing.T, branch string) (string, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	gitIn := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, out)
		}
	}

	gitIn(repo, "init", "-q", "-b", branch)
	gitIn(repo, "config", "user.email", "t@t")
	gitIn(repo, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(repo, "add", "f")
	gitIn(repo, "commit", "-q", "-m", "v1")

	// Create the bare remote, push the branch.
	gitIn(repo, "init", "--bare", "-q", remote)
	gitIn(repo, "remote", "add", "origin", remote)
	gitIn(repo, "push", "-q", "-u", "origin", branch)

	return repo, remote
}

// advanceRemote adds a new commit to `branch` on the remote without touching
// the local repo. Done by cloning into a scratch worktree.
func advanceRemote(t *testing.T, remote, branch string) {
	t.Helper()
	scratch := filepath.Join(t.TempDir(), "scratch")
	gitIn := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, out)
		}
	}
	c := exec.Command("git", "clone", "-q", "-b", branch, remote, scratch)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(scratch, "f"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(scratch, "config", "user.email", "t@t")
	gitIn(scratch, "config", "user.name", "test")
	gitIn(scratch, "commit", "-q", "-am", "v2")
	gitIn(scratch, "push", "-q", "origin", branch)
}

func TestPullBranch_FastForward(t *testing.T) {
	repo, remote := setupRepoWithRemote(t, "main")
	t.Chdir(repo)

	r := git.Root()
	before, _ := r.RevParse("main")
	advanceRemote(t, remote, "main")
	if err := pullBranch(r, "main"); err != nil {
		t.Fatalf("pullBranch: %v", err)
	}
	after, _ := r.RevParse("main")
	if after == before {
		t.Errorf("local main did not advance; before=%s after=%s", before, after)
	}
	upstream, _ := r.RevParse("origin/main")
	if after != upstream {
		t.Errorf("local main %s != origin/main %s", after, upstream)
	}
}

func TestPullBranch_NoUpstream(t *testing.T) {
	repo, _ := setupRepoWithRemote(t, "main")
	t.Chdir(repo)
	// Create a local-only branch (no upstream tracking).
	c := exec.Command("git", "branch", "feat/local-only", "main")
	c.Dir = repo
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	r := git.Root()
	before, _ := r.RevParse("feat/local-only")
	if err := pullBranch(r, "feat/local-only"); err != nil {
		t.Errorf("pullBranch with no upstream should skip silently, got %v", err)
	}
	after, _ := r.RevParse("feat/local-only")
	if after != before {
		t.Errorf("branch tip changed despite no upstream: %s -> %s", before, after)
	}
}

func TestPullBranch_AlreadyUpToDate(t *testing.T) {
	repo, _ := setupRepoWithRemote(t, "main")
	t.Chdir(repo)

	r := git.Root()
	before, _ := r.RevParse("main")
	if err := pullBranch(r, "main"); err != nil {
		t.Fatalf("pullBranch: %v", err)
	}
	after, _ := r.RevParse("main")
	if after != before {
		t.Errorf("up-to-date branch should not move: %s -> %s", before, after)
	}
}

func TestPullBranch_Diverged(t *testing.T) {
	repo, remote := setupRepoWithRemote(t, "main")
	t.Chdir(repo)

	gitIn := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// Local commit on top of main.
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn("commit", "-q", "-am", "local-only")
	// Diverging commit on the remote.
	advanceRemote(t, remote, "main")

	err := pullBranch(git.Root(), "main")
	if err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Errorf("expected diverge error, got %v", err)
	}
}
