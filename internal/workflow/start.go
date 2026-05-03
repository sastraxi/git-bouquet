package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sastraxi/git-bouquet/internal/git"
	"github.com/sastraxi/git-bouquet/internal/leaves"
	"github.com/sastraxi/git-bouquet/internal/state"
)

type StartOpts struct {
	Pull   bool
	Sync   bool
	DryRun bool
}

func Start(opts StartOpts) error {
	env, err := Setup()
	if err != nil {
		return err
	}

	existing, err := state.Load(env.Paths)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("a rebuild is already in progress (state at %s) — use `git bouquet continue` or `git bouquet abort`", env.Paths.StateFile)
	}

	if err := bootstrapRerere(env.Repo); err != nil {
		return err
	}

	if opts.Pull {
		if err := pullBranch(env.Repo, env.Config.Base); err != nil {
			return err
		}
	}

	resolved, err := leaves.Resolve(env.Repo, env.Config.Merge, []string{env.Config.Target, env.Config.Base})
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		return fmt.Errorf("no branches matched any pattern in `merge`")
	}

	if opts.Pull {
		for _, b := range resolved {
			if err := pullBranch(env.Repo, b); err != nil {
				return err
			}
		}
	}
	if opts.Sync {
		if err := ensureGitTown(); err != nil {
			return err
		}
		for _, b := range resolved {
			if err := syncBranch(env.Repo, b); err != nil {
				return err
			}
		}
	}

	// Re-resolve after pull/sync may have updated tips (and possibly created
	// new ancestor relationships).
	resolved, err = leaves.Resolve(env.Repo, env.Config.Merge, []string{env.Config.Target, env.Config.Base})
	if err != nil {
		return err
	}

	baseSHA, err := env.Repo.RevParse(env.Config.Base)
	if err != nil {
		return err
	}
	if baseSHA == "" {
		return fmt.Errorf("base branch %q does not exist", env.Config.Base)
	}
	prevTargetSHA, err := env.Repo.RevParse(env.Config.Target)
	if err != nil {
		return err
	}

	leafSHAs := make(map[string]string, len(resolved))
	for _, b := range resolved {
		sha, err := env.Repo.RevParse(b)
		if err != nil {
			return err
		}
		if sha == "" {
			return fmt.Errorf("leaf %q does not exist (was it deleted between resolve and rev-parse?)", b)
		}
		leafSHAs[b] = sha
	}

	st := &state.State{
		Target:        env.Config.Target,
		Base:          env.Config.Base,
		BaseSHA:       baseSHA,
		PrevTargetSHA: prevTargetSHA,
		Leaves:        resolved,
		NextIndex:     0,
		LeafSHAs:      leafSHAs,
	}
	if err := state.Save(env.Paths, st); err != nil {
		return err
	}

	if err := setupWorktree(env, baseSHA); err != nil {
		return err
	}

	info("merging %d leaves into %s:", len(resolved), env.Config.Target)
	return runMergeLoop(env, st, opts.DryRun)
}

func bootstrapRerere(repo *git.Repo) error {
	val, err := repo.ConfigGet("rerere.enabled")
	if err != nil {
		return err
	}
	if val == "true" || val == "1" {
		return nil
	}
	if err := repo.ConfigSet("rerere.enabled", "true"); err != nil {
		return err
	}
	info("enabled rerere.enabled at repo scope")
	return nil
}

func pullBranch(repo *git.Repo, branch string) error {
	info("pull --ff-only %s", branch)
	// Use `git fetch` + `git update-ref` instead of `git pull` so we don't
	// need to check out the branch. This works whether or not it's checked
	// out anywhere.
	upstream, err := repo.Output("for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch)
	if err != nil {
		return err
	}
	if upstream == "" {
		warn("  %s has no upstream — skipping", branch)
		return nil
	}
	remote := strings.SplitN(upstream, "/", 2)[0]
	if err := repo.Run("fetch", remote, branch); err != nil {
		return fmt.Errorf("fetch %s %s: %w", remote, branch, err)
	}
	// Fast-forward the local branch ref to the upstream tip.
	upstreamSHA, err := repo.RevParse(upstream)
	if err != nil {
		return err
	}
	localSHA, err := repo.RevParse(branch)
	if err != nil {
		return err
	}
	if upstreamSHA == localSHA {
		return nil
	}
	isAnc, err := repo.IsAncestor(localSHA, upstreamSHA)
	if err != nil {
		return err
	}
	if !isAnc {
		return fmt.Errorf("cannot fast-forward %s: local has diverged from %s", branch, upstream)
	}
	return repo.Quiet("update-ref", "refs/heads/"+branch, upstreamSHA, localSHA)
}

func ensureGitTown() error {
	_, err := exec.LookPath("git-town")
	if err != nil {
		return fmt.Errorf("--sync requested but `git-town` is not on PATH")
	}
	return nil
}

func syncBranch(repo *git.Repo, branch string) error {
	info("git town sync -s %s", branch)
	return repo.Run("town", "sync", "-s", branch)
}

func setupWorktree(env *Env, baseSHA string) error {
	// If a stale worktree exists (e.g. from a crashed prior run), remove it.
	if _, err := os.Stat(env.Paths.WorktreeDir); err == nil {
		_ = env.Repo.Quiet("worktree", "remove", "--force", env.Paths.WorktreeDir)
		_ = os.RemoveAll(env.Paths.WorktreeDir)
	}
	if err := env.Repo.Quiet("worktree", "add", "--detach", env.Paths.WorktreeDir, baseSHA); err != nil {
		return fmt.Errorf("creating worktree at %s: %w", env.Paths.WorktreeDir, err)
	}
	return nil
}

func runMergeLoop(env *Env, st *state.State, dryRun bool) error {
	wt := env.Repo.In(env.Paths.WorktreeDir)
	for st.NextIndex < len(st.Leaves) {
		leaf := st.Leaves[st.NextIndex]
		fmt.Printf("  %s... ", leaf)
		err := wt.Run("merge", "--no-ff", "--no-edit", leaf)
		if err != nil {
			unmerged, uerr := wt.HasUnmergedPaths()
			if uerr != nil {
				return uerr
			}
			if unmerged {
				fmt.Println("CONFLICT")
				warn("\nResolve conflicts in %s, `git add` them, then run `git bouquet continue`.", env.Paths.WorktreeDir)
				return errExitConflict
			}
			return fmt.Errorf("merging %s: %w", leaf, err)
		}
		fmt.Println("ok")
		st.NextIndex++
		if err := state.Save(env.Paths, st); err != nil {
			return err
		}
	}
	return finishRebuild(env, st, dryRun)
}

func finishRebuild(env *Env, st *state.State, dryRun bool) error {
	wt := env.Repo.In(env.Paths.WorktreeDir)

	if dryRun {
		info("\n--dry-run: skipping commit on %s", st.Target)
		// Leave worktree + state in place so the user can poke at it. They
		// can `git bouquet abort` to clean.
		return nil
	}

	tree, err := wt.Output("rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	parent := st.PrevTargetSHA
	if parent == "" {
		parent = st.BaseSHA
	}
	msg := buildCommitMessage(st)
	newSHA, err := env.Repo.Output("commit-tree", tree, "-p", parent, "-m", msg)
	if err != nil {
		return fmt.Errorf("commit-tree: %w", err)
	}
	if st.PrevTargetSHA == "" {
		if err := env.Repo.Quiet("update-ref", "refs/heads/"+st.Target, newSHA); err != nil {
			return err
		}
	} else {
		if err := env.Repo.Quiet("update-ref", "refs/heads/"+st.Target, newSHA, st.PrevTargetSHA); err != nil {
			return err
		}
	}

	if err := env.Repo.Quiet("worktree", "remove", "--force", env.Paths.WorktreeDir); err != nil {
		warn("worktree cleanup failed: %v", err)
	}
	if err := state.Clear(env.Paths); err != nil {
		warn("state cleanup failed: %v", err)
	}

	short := newSHA
	if len(short) > 12 {
		short = short[:12]
	}
	parentShort := parent
	if len(parentShort) > 12 {
		parentShort = parentShort[:12]
	}
	info("\ncommitted %s %s (parent %s)", st.Target, short, parentShort)
	return nil
}

func buildCommitMessage(st *state.State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bouquet: rebuild %s from %s + %d leaves\n\n",
		time.Now().UTC().Format("2006-01-02T15:04Z"), st.Base, len(st.Leaves))
	fmt.Fprintf(&b, "base: %s @ %s\n", st.Base, shortSHA(st.BaseSHA))
	b.WriteString("leaves:\n")
	for _, l := range st.Leaves {
		fmt.Fprintf(&b, "  %s @ %s\n", l, shortSHA(st.LeafSHAs[l]))
	}
	return b.String()
}

func shortSHA(s string) string {
	if len(s) >= 12 {
		return s[:12]
	}
	return s
}
