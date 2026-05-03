package workflow

import (
	"fmt"

	"github.com/sastraxi/git-bouquet/internal/state"
)

func Status() error {
	env, err := Setup()
	if err != nil {
		return err
	}
	st, err := state.Load(env.Paths)
	if err != nil {
		return err
	}
	if st == nil {
		info("no rebuild in progress")
		info("config: target=%s base=%s", env.Config.Target, env.Config.Base)
		return nil
	}
	info("rebuild in progress for %s (base %s @ %s)", st.Target, st.Base, shortSHA(st.BaseSHA))
	info("worktree: %s", env.Paths.WorktreeDir)
	info("progress: %d/%d leaves merged", st.NextIndex, len(st.Leaves))
	for i, l := range st.Leaves {
		marker := "  "
		switch {
		case i < st.NextIndex:
			marker = "✓ "
		case i == st.NextIndex:
			marker = "→ "
		}
		fmt.Printf("  %s%s @ %s\n", marker, l, shortSHA(st.LeafSHAs[l]))
	}
	return nil
}
