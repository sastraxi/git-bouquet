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
	body := "target: " + target + "\nbase: " + base + "\nmerge:\n"
	for _, m := range merge {
		body += "  - " + m + "\n"
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

	if err := Start(StartOpts{}); err != nil {
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

	if err := Start(StartOpts{}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	sha1 := mustHead(t, "release/current")

	if err := Start(StartOpts{}); err != nil {
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

	if err := Start(StartOpts{}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	tree1, _ := git.Root().Output("rev-parse", "release/current^{tree}")

	if err := Start(StartOpts{}); err != nil {
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

	if err := Start(StartOpts{DryRun: true}); err != nil {
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

	err := Start(StartOpts{})
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

	err := Start(StartOpts{})
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

	if err := Start(StartOpts{}); !IsConflict(err) {
		t.Fatalf("setup: expected conflict, got %v", err)
	}
	defer Abort()

	err := Start(StartOpts{})
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

	if err := List(); err != nil {
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
	if err := Start(StartOpts{}); !IsConflict(err) {
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
	if err := Start(StartOpts{}); err != nil {
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

	if err := Start(StartOpts{}); !IsConflict(err) {
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

	if err := Start(StartOpts{}); !IsConflict(err) {
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
	if err := Start(StartOpts{}); err != nil {
		t.Fatalf("Start should auto-resolve DU conflict, got: %v", err)
	}
	if out, err := exec.Command("git", "show", "release/current:old.py").Output(); err == nil {
		t.Errorf("old.py should be absent from snapshot, got: %s", out)
	}
}

func TestStart_DeletedByThem_AutoResolved(t *testing.T) {
	// UD: modifier merged first → worktree has old.py, deleter branch removes it.
	_ = modifyDeleteRepo(t, false)
	if err := Start(StartOpts{}); err != nil {
		t.Fatalf("Start should auto-resolve UD conflict, got: %v", err)
	}
	if out, err := exec.Command("git", "show", "release/current:old.py").Output(); err == nil {
		t.Errorf("old.py should be absent from snapshot, got: %s", out)
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

