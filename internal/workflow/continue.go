package workflow

import (
	"fmt"

	"github.com/sastraxi/git-bouquet/internal/state"
)

func Continue() error {
	env, err := Setup()
	if err != nil {
		return err
	}
	st, err := state.Load(env.Paths)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("no rebuild in progress")
	}
	wt := env.Repo.In(env.Paths.WorktreeDir)

	unmerged, err := wt.HasUnmergedPaths()
	if err != nil {
		return err
	}
	if unmerged {
		warn("There are still unmerged paths in %s.", env.Paths.WorktreeDir)
		out, _ := wt.Output("diff", "--name-only", "--diff-filter=U")
		warn("%s", out)
		return errExitConflict
	}

	inMerge, err := wt.MergeInProgress()
	if err != nil {
		return err
	}
	if inMerge {
		// Seal the merge with the default message.
		if err := wt.Run("commit", "--no-edit"); err != nil {
			return fmt.Errorf("finalizing merge commit: %w", err)
		}
		// The leaf we were merging has now been committed; advance past it.
		if st.NextIndex < len(st.Leaves) {
			leaf := st.Leaves[st.NextIndex]
			fmt.Printf("  %s... resolved\n", leaf)
			st.NextIndex++
			if err := state.Save(env.Paths, st); err != nil {
				return err
			}
		}
	}

	return runMergeLoop(env, st, false)
}
