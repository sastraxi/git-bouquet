// Package leaves expands merge globs into a deterministic, ancestor-trimmed,
// alphabetically-sorted leaf list.
package leaves

import (
	"fmt"
	"sort"

	"github.com/gobwas/glob"

	"github.com/sastraxi/git-bouquet/internal/git"
)

// Resolve expands the given glob patterns against local branches, drops
// branches matching `excludes`, removes ancestors (if A is an ancestor of B
// and both are in the set, drop A), and sorts the result alphabetically.
//
// Glob semantics: gobwas/glob with no separator, so `*` crosses `/` to match
// git refspec conventions (e.g. `feat/*` matches `feat/a/b`).
func Resolve(repo *git.Repo, patterns, excludes []string) ([]string, error) {
	branches, err := repo.LocalBranches()
	if err != nil {
		return nil, fmt.Errorf("listing local branches: %w", err)
	}
	excludeSet := make(map[string]struct{}, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = struct{}{}
	}

	matched := map[string]struct{}{}
	for _, p := range patterns {
		g, err := glob.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", p, err)
		}
		for _, b := range branches {
			if _, skip := excludeSet[b]; skip {
				continue
			}
			if g.Match(b) {
				matched[b] = struct{}{}
			}
		}
	}

	list := make([]string, 0, len(matched))
	for b := range matched {
		list = append(list, b)
	}
	sort.Strings(list)

	trimmed, err := trimAncestors(repo, list)
	if err != nil {
		return nil, err
	}
	sort.Strings(trimmed)
	return trimmed, nil
}

// trimAncestors removes any branch X for which there exists another branch Y
// in the set such that X is an ancestor of Y (merging Y subsumes X).
func trimAncestors(repo *git.Repo, branches []string) ([]string, error) {
	keep := make([]bool, len(branches))
	for i := range keep {
		keep[i] = true
	}
	for i, x := range branches {
		if !keep[i] {
			continue
		}
		for j, y := range branches {
			if i == j || !keep[j] {
				continue
			}
			anc, err := repo.IsAncestor(x, y)
			if err != nil {
				return nil, fmt.Errorf("merge-base %s %s: %w", x, y, err)
			}
			if anc {
				keep[i] = false
				break
			}
		}
	}
	out := branches[:0:0]
	for i, b := range branches {
		if keep[i] {
			out = append(out, b)
		}
	}
	return out, nil
}
