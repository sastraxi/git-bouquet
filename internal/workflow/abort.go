package workflow

import (
	"fmt"
	"os"

	"github.com/sastraxi/git-bouquet/internal/state"
)

func Abort() error {
	env, err := Setup()
	if err != nil {
		return err
	}
	st, err := state.Load(env.Paths)
	if err != nil {
		return err
	}
	if st == nil && !pathExists(env.Paths.WorktreeDir) {
		return fmt.Errorf("no rebuild in progress")
	}

	if pathExists(env.Paths.WorktreeDir) {
		if err := env.Repo.Quiet("worktree", "remove", "--force", env.Paths.WorktreeDir); err != nil {
			warn("worktree remove failed: %v", err)
			_ = os.RemoveAll(env.Paths.WorktreeDir)
		}
	}
	if err := state.Clear(env.Paths); err != nil {
		return err
	}
	info("aborted; integration branch left untouched")
	return nil
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
