package workflow

import (
	"fmt"

	"github.com/sastraxi/git-bouquet/internal/leaves"
)

func List(target string) error {
	env, err := Setup()
	if err != nil {
		return err
	}

	if target == "" {
		if len(env.Config.Branches) == 1 {
			for t := range env.Config.Branches {
				target = t
			}
		} else {
			info("base: %s", env.Config.Base)
			info("targets:")
			for t := range env.Config.Branches {
				info("  %s", t)
			}
			return nil
		}
	}

	mergeGlobs, ok := env.Config.Branches[target]
	if !ok {
		return fmt.Errorf("target branch %q not found in config", target)
	}

	resolved, err := leaves.Resolve(env.Repo, mergeGlobs, []string{target, env.Config.Base})
	if err != nil {
		return err
	}
	info("target: %s", target)
	info("base:   %s", env.Config.Base)
	info("merge order (%d leaves):", len(resolved))
	for _, b := range resolved {
		sha, _ := env.Repo.RevParse(b)
		fmt.Printf("  %s @ %s\n", b, shortSHA(sha))
	}
	return nil
}
