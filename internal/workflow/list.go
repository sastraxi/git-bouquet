package workflow

import (
	"fmt"

	"github.com/sastraxi/git-bouquet/internal/leaves"
)

func List() error {
	env, err := Setup()
	if err != nil {
		return err
	}
	resolved, err := leaves.Resolve(env.Repo, env.Config.Merge, []string{env.Config.Target, env.Config.Base})
	if err != nil {
		return err
	}
	info("target: %s", env.Config.Target)
	info("base:   %s", env.Config.Base)
	info("merge order (%d leaves):", len(resolved))
	for _, b := range resolved {
		sha, _ := env.Repo.RevParse(b)
		fmt.Printf("  %s @ %s\n", b, shortSHA(sha))
	}
	return nil
}
