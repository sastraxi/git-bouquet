package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sastraxi/git-bouquet/internal/git"
	"github.com/sastraxi/git-bouquet/internal/state"
)

// setupRepo creates a tmp git repo, makes an initial commit on `base`, then
// creates each branch in `branches` off `base` with a single commit
// containing the given files. Returns the absolute repo path.
//
// The test is chdir'd into the repo for the duration; t.Chdir restores cwd.
func setupRepo(t *testing.T, base string, branches map[string]map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init", "-q", "-b", base)
	run("config", "user.email", "t@t")
	run("config", "user.name", "test")
	run("config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-q", "-m", "base init")

	for branch, files := range branches {
		run("checkout", "-q", "-b", branch, base)
		for path, content := range files {
			full := filepath.Join(dir, path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			run("add", path)
		}
		run("commit", "-q", "-m", "branch "+branch)
	}
	run("checkout", "-q", base)
	return dir
}

func writeBouquetYAML(t *testing.T, dir, target, base string, merge []string) {
	t.Helper()
	body := "base: " + base + "\nbranches:\n  " + target + ":\n"
	for _, m := range merge {
		body += "    - " + m + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".bouquet.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustHead(t *testing.T, branch string) string {
	t.Helper()
	sha, err := git.Root().RevParse(branch)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", branch, err)
	}
	return sha
}

func TestStart_HappyPath(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "alpha\n"},
		"feat/b": {"b.txt": "bravo\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	target := mustHead(t, "release/current")
	if target == "" {
		t.Fatal("release/current was not created")
	}

	// Both files should be present in the snapshot.
	for _, f := range []string{"a.txt", "b.txt", "README"} {
		out, err := exec.Command("git", "show", "release/current:"+f).Output()
		if err != nil {
			t.Errorf("missing %s in snapshot: %v", f, err)
			continue
		}
		if len(out) == 0 {
			t.Errorf("%s empty in snapshot", f)
		}
	}

	// State should be cleaned up.
	gitDir, _ := git.Root().GitDir()
	p := state.Locate(gitDir)
	if _, err := os.Stat(p.WorktreeDir); !os.IsNotExist(err) {
		t.Errorf("worktree should be gone, stat err=%v", err)
	}
	if _, err := os.Stat(p.StateFile); !os.IsNotExist(err) {
		t.Errorf("state file should be gone, stat err=%v", err)
	}

	// Snapshot commit should have base as parent (first run).
	parent, err := git.Root().Output("rev-parse", "release/current^1")
	if err != nil {
		t.Fatal(err)
	}
	if parent != mustHead(t, "main") {
		t.Errorf("snapshot parent = %s, want main tip %s", parent, mustHead(t, "main"))
	}
}

func TestStart_NoopWhenTreeUnchanged(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "alpha\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	sha1 := mustHead(t, "release/current")

	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	sha2 := mustHead(t, "release/current")

	if sha1 != sha2 {
		t.Errorf("release/current moved from %s to %s despite no change", sha1, sha2)
	}
}

func TestStart_Idempotent(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "alpha\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	tree1, _ := git.Root().Output("rev-parse", "release/current^{tree}")

	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	tree2, _ := git.Root().Output("rev-parse", "release/current^{tree}")

	if tree1 != tree2 {
		t.Errorf("tree differs across rebuilds: %s vs %s", tree1, tree2)
	}
}

func TestStart_DryRun_DoesNotUpdateTarget(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "alpha\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{DryRun: true}); err != nil {
		t.Fatalf("Start dry-run: %v", err)
	}
	if sha := mustHead(t, "release/current"); sha != "" {
		t.Errorf("release/current should not exist after dry-run, got %s", sha)
	}
	// Worktree should still exist (dry-run leaves it for inspection).
	gitDir, _ := git.Root().GitDir()
	p := state.Locate(gitDir)
	if _, err := os.Stat(p.WorktreeDir); err != nil {
		t.Errorf("worktree should remain after dry-run: %v", err)
	}
	// Clean up for the next test.
	if err := Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestStart_Conflict_then_Continue(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"shared.txt": "from-a\n"},
		"feat/b": {"shared.txt": "from-b\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	err := Start("", StartOpts{})
	if !IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// Resolve the conflict in the worktree by picking feat/b's version.
	gitDir, _ := git.Root().GitDir()
	p := state.Locate(gitDir)
	wt := git.Root().In(p.WorktreeDir)
	if err := os.WriteFile(filepath.Join(p.WorktreeDir, "shared.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := wt.Quiet("add", "shared.txt"); err != nil {
		t.Fatal(err)
	}

	if err := Continue(); err != nil {
		t.Fatalf("Continue: %v", err)
	}

	// Snapshot should contain the resolved content.
	out, err := exec.Command("git", "show", "release/current:shared.txt").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "resolved\n" {
		t.Errorf("shared.txt = %q, want %q", out, "resolved\n")
	}

	// State should be cleared.
	if _, err := os.Stat(p.StateFile); !os.IsNotExist(err) {
		t.Errorf("state file should be gone after Continue completes")
	}

	_ = dir
}

func TestStart_Conflict_then_Abort(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"shared.txt": "from-a\n"},
		"feat/b": {"shared.txt": "from-b\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	err := Start("", StartOpts{})
	if !IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	gitDir, _ := git.Root().GitDir()
	p := state.Locate(gitDir)
	if _, err := os.Stat(p.StateFile); !os.IsNotExist(err) {
		t.Errorf("state file should be gone after Abort")
	}
	if _, err := os.Stat(p.WorktreeDir); !os.IsNotExist(err) {
		t.Errorf("worktree should be gone after Abort")
	}
	// release/current should never have been created.
	if sha := mustHead(t, "release/current"); sha != "" {
		t.Errorf("release/current should not exist after Abort, got %s", sha)
	}
	_ = dir
}

func TestStart_RefusesWhenStateExists(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"shared.txt": "from-a\n"},
		"feat/b": {"shared.txt": "from-b\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{}); !IsConflict(err) {
		t.Fatalf("setup: expected conflict, got %v", err)
	}
	defer Abort()

	err := Start("", StartOpts{})
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("expected 'already in progress' error, got %v", err)
	}
	_ = dir
}

func TestList_AfterTrim(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "a\n"},
	})
	// feat/b is built on feat/a (so feat/a should be trimmed as ancestor).
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("checkout", "-q", "-b", "feat/b", "feat/a")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "b.txt")
	run("commit", "-q", "-m", "b on top of a")
	run("checkout", "-q", "main")

	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := List(""); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestStatus_NoRebuild(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "a\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})
	if err := Status(); err != nil {
		t.Fatalf("Status: %v", err)
	}
	_ = dir
}

func TestAbort_NothingToDo(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "a\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})
	err := Abort()
	if err == nil || !strings.Contains(err.Error(), "no rebuild in progress") {
		t.Errorf("expected 'no rebuild in progress', got %v", err)
	}
	_ = dir
}

// TestStart_RerereReplay is the central correctness test: after a conflict
// is resolved once, a subsequent rebuild against the same inputs must
// auto-apply the recorded resolution and complete without manual
// intervention. If this ever breaks, the tool's value proposition breaks.
func TestStart_RerereReplay(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"shared.txt": "from-a\n"},
		"feat/b": {"shared.txt": "from-b\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	// First rebuild: hit conflict, resolve, continue.
	if err := Start("", StartOpts{}); !IsConflict(err) {
		t.Fatalf("first Start: expected conflict, got %v", err)
	}
	gitDir, _ := git.Root().GitDir()
	p := state.Locate(gitDir)
	if err := os.WriteFile(filepath.Join(p.WorktreeDir, "shared.txt"), []byte("rerere-resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := git.Root().In(p.WorktreeDir).Quiet("add", "shared.txt"); err != nil {
		t.Fatal(err)
	}
	if err := Continue(); err != nil {
		t.Fatalf("Continue: %v", err)
	}

	// Reset target so the second rebuild has to redo the merges from scratch.
	if err := git.Root().Quiet("update-ref", "-d", "refs/heads/release/current"); err != nil {
		t.Fatal(err)
	}

	// Second rebuild: same conflict recurs, but rerere should auto-apply.
	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("second Start should complete via rerere replay, got: %v", err)
	}

	// Snapshot must contain the same resolved content.
	out, err := exec.Command("git", "show", "release/current:shared.txt").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "rerere-resolved\n" {
		t.Errorf("rerere did not replay the resolution: shared.txt = %q", out)
	}
}

func TestStatus_InProgress(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"shared.txt": "a\n"},
		"feat/b": {"shared.txt": "b\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{}); !IsConflict(err) {
		t.Fatalf("setup: expected conflict, got %v", err)
	}
	defer Abort()

	if err := Status(); err != nil {
		t.Fatalf("Status during in-progress rebuild: %v", err)
	}
	_ = dir
}

func TestContinue_StillUnmerged(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"shared.txt": "a\n"},
		"feat/b": {"shared.txt": "b\n"},
	})
	writeBouquetYAML(t, dir, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{}); !IsConflict(err) {
		t.Fatalf("setup: expected conflict, got %v", err)
	}
	defer Abort()

	// Don't resolve — call Continue immediately.
	err := Continue()
	if !IsConflict(err) {
		t.Errorf("Continue with unresolved paths should signal conflict, got %v", err)
	}
	_ = dir
}

// modifyDeleteRepo builds a repo where one branch deletes old.py and another
// modifies it. deleterFirst controls merge order: if true, the deleting branch
// is merged first (DU conflict on the second merge); if false, the modifier is
// first (UD conflict on the second merge).
func modifyDeleteRepo(t *testing.T, deleterFirst bool) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "test")
	run("config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(dir, "old.py"), []byte("old content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "old.py")
	run("commit", "-q", "-m", "base init")

	run("checkout", "-q", "-b", "feat/deleter", "main")
	run("rm", "old.py")
	run("commit", "-q", "-m", "feat/deleter: remove old.py")

	run("checkout", "-q", "-b", "feat/modifier", "main")
	if err := os.WriteFile(filepath.Join(dir, "old.py"), []byte("modified content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "old.py")
	run("commit", "-q", "-m", "feat/modifier: modify old.py")

	run("checkout", "-q", "main")
	merge := []string{"feat/deleter", "feat/modifier"}
	if !deleterFirst {
		merge = []string{"feat/modifier", "feat/deleter"}
	}
	writeBouquetYAML(t, dir, "release/current", "main", merge)
	return dir
}

func TestStart_DeletedByUs_AutoResolved(t *testing.T) {
	// DU: deleter merged first → worktree has no old.py, modifier tries to add changes.
	_ = modifyDeleteRepo(t, true)
	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("Start should auto-resolve DU conflict, got: %v", err)
	}
	if out, err := exec.Command("git", "show", "release/current:old.py").Output(); err == nil {
		t.Errorf("old.py should be absent from snapshot, got: %s", out)
	}
}

func TestStart_DeletedByThem_AutoResolved(t *testing.T) {
	// UD: modifier merged first → worktree has old.py, deleter branch removes it.
	_ = modifyDeleteRepo(t, false)
	if err := Start("", StartOpts{}); err != nil {
		t.Fatalf("Start should auto-resolve UD conflict, got: %v", err)
	}
	if out, err := exec.Command("git", "show", "release/current:old.py").Output(); err == nil {
		t.Errorf("old.py should be absent from snapshot, got: %s", out)
	}
}

// TestStart_Pull_DoesNotDirtyMainWorktree is the regression test for the bug
// where pullBranch fast-forwards a branch ref via git-update-ref without
// touching the working tree or index. When the pulled branch is the currently
// checked-out branch, HEAD moves forward but the files on disk stay at the old
// SHA, leaving the working tree dirty after an otherwise-successful bouquet
// commit.
func TestStart_Pull_DoesNotDirtyMainWorktree(t *testing.T) {
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

	// Advance remote main first, then fetch so we can branch feat/a off the
	// new tip. This ensures feat/a is a descendant of the post-pull base SHA
	// (which is required by the leaf-ancestry constraint).
	advanceRemote(t, remote, "main")
	gitIn("fetch", "-q", "origin")

	// Create feat/a off FETCH_HEAD (the new remote main tip, SHA2).
	// Local main is still at SHA1 — that's the whole point: Start --pull must
	// advance local main to SHA2 AND update the working tree to match.
	gitIn("checkout", "-q", "-b", "feat/a", "FETCH_HEAD")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn("add", "a.txt")
	gitIn("commit", "-q", "-m", "feat/a")
	gitIn("checkout", "-q", "main")

	writeBouquetYAML(t, repo, "release/current", "main", []string{"feat/*"})

	if err := Start("", StartOpts{Pull: true}); err != nil {
		t.Fatalf("Start --pull: %v", err)
	}

	// Working tree must have no changes to tracked files. We filter out
	// untracked lines (??) because .bouquet.yaml is intentionally untracked
	// in tests; the bug we're guarding is that pullBranch leaves tracked
	// files stale (e.g. "M  f") after fast-forwarding the checked-out branch.
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	var dirty []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.HasPrefix(line, "??") && strings.TrimSpace(line) != "" {
			dirty = append(dirty, line)
		}
	}
	if len(dirty) > 0 {
		t.Errorf("tracked files dirty after successful Start --pull:\n%s", strings.Join(dirty, "\n"))
	}
}

func TestStart_LeafNotDescendantOfBase_Rejected(t *testing.T) {
	// Create a repo where one leaf branches off a different commit (not base),
	// simulating e.g. a branch accidentally rooted on main instead of the
	// release base. git bouquet must reject this before touching the worktree.
	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "test")
	run("config", "commit.gpgsign", "false")

	// main commit — this is NOT the bouquet base.
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-q", "-m", "main init")

	// base branches off main.
	run("checkout", "-q", "-b", "base", "main")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "base.txt")
	run("commit", "-q", "-m", "base commit")

	// feat/good branches off base — this is fine.
	run("checkout", "-q", "-b", "feat/good", "base")
	if err := os.WriteFile(filepath.Join(dir, "good.txt"), []byte("good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "good.txt")
	run("commit", "-q", "-m", "feat/good")

	// feat/bad branches off main, NOT base — this should be rejected.
	run("checkout", "-q", "-b", "feat/bad", "main")
	if err := os.WriteFile(filepath.Join(dir, "bad.txt"), []byte("bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "bad.txt")
	run("commit", "-q", "-m", "feat/bad rooted on main, not base")

	run("checkout", "-q", "base")
	writeBouquetYAML(t, dir, "release/current", "base", []string{"feat/*"})

	err := Start("", StartOpts{})
	if err == nil || !strings.Contains(err.Error(), "not a descendant of base") {
		t.Errorf("expected 'not a descendant of base' error, got: %v", err)
	}

	// Worktree must not have been created — validation should fail before setup.
	gitDir, _ := git.Root().GitDir()
	p := state.Locate(gitDir)
	if _, err := os.Stat(p.WorktreeDir); !os.IsNotExist(err) {
		t.Errorf("worktree should not exist after early validation failure")
		_ = Abort()
	}
}

func TestStart_MultipleBranches(t *testing.T) {
	dir := setupRepo(t, "main", map[string]map[string]string{
		"feat/a": {"a.txt": "alpha\n"},
		"feat/b": {"b.txt": "bravo\n"},
	})
	
	// Write config with two targets
	body := "base: main\nbranches:\n  target1:\n    - feat/a\n  target2:\n    - feat/b\n"
	if err := os.WriteFile(filepath.Join(dir, ".bouquet.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Starting without target should fail
	if err := Start("", StartOpts{}); err == nil || !strings.Contains(err.Error(), "please specify a target branch") {
		t.Errorf("expected error for missing target with multiple branches, got %v", err)
	}

	// Start target1
	if err := Start("target1", StartOpts{}); err != nil {
		t.Fatalf("Start target1: %v", err)
	}
	if sha := mustHead(t, "target1"); sha == "" {
		t.Fatal("target1 not created")
	}

	// Start target2
	if err := Start("target2", StartOpts{}); err != nil {
		t.Fatalf("Start target2: %v", err)
	}
	if sha := mustHead(t, "target2"); sha == "" {
		t.Fatal("target2 not created")
	}
}

func TestSetup_NoConfig(t *testing.T) {
	// A bare git repo with no .bouquet.yaml.
	dir := t.TempDir()
	t.Chdir(dir)
	c := exec.Command("git", "init", "-q")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	_, err := Setup()
	if err == nil || !strings.Contains(err.Error(), "no .bouquet.yaml") {
		t.Errorf("expected 'no .bouquet.yaml' error, got %v", err)
	}
}

