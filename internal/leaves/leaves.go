// Package leaves expands merge globs into a deterministic, ancestor-trimmed,
// alphabetically-sorted leaf list.
package leaves

import (
	"fmt"
	"sort"

	"github.com/gobwas/glob"
)

// Source is the minimal git surface area Resolve needs. *git.Repo satisfies
// it implicitly; tests provide an in-memory fake.
type Source interface {
	LocalBranches() ([]string, error)
	IsAncestor(a, b string) (bool, error)
}

// Resolve expands the given glob patterns against local branches, drops
// branches matching `excludes`, removes ancestors (if A is an ancestor of B
// and both are in the set, drop A), and sorts the result alphabetically.
//
// Glob semantics: gobwas/glob with no separator, so `*` crosses `/` to match
// git refspec conventions (e.g. `feat/*` matches `feat/a/b`). Patterns
// prefixed with `!` are negative — any branch matching a negative pattern
// is dropped from the result regardless of include matches.
func Resolve(src Source, patterns, excludes []string) ([]string, error) {
	branches, err := src.LocalBranches()
	if err != nil {
		return nil, fmt.Errorf("listing local branches: %w", err)
	}
	excludeSet := make(map[string]struct{}, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = struct{}{}
	}

	var includes, negatives []glob.Glob
	for _, p := range patterns {
		neg := false
		raw := p
		if len(raw) > 0 && raw[0] == '!' {
			neg = true
			raw = raw[1:]
		}
		g, err := glob.Compile(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", p, err)
		}
		if neg {
			negatives = append(negatives, g)
		} else {
			includes = append(includes, g)
		}
	}

	matched := map[string]struct{}{}
	for _, b := range branches {
		if _, skip := excludeSet[b]; skip {
			continue
		}
		negated := false
		for _, ng := range negatives {
			if ng.Match(b) {
				negated = true
				break
			}
		}
		if negated {
			continue
		}
		for _, ig := range includes {
			if ig.Match(b) {
				matched[b] = struct{}{}
				break
			}
		}
	}

	list := make([]string, 0, len(matched))
	for b := range matched {
		list = append(list, b)
	}
	sort.Strings(list)

	trimmed, err := trimAncestors(src, list)
	if err != nil {
		return nil, err
	}
	sort.Strings(trimmed)
	return trimmed, nil
}

// trimAncestors removes any branch X for which there exists another branch Y
// in the set such that X is an ancestor of Y (merging Y subsumes X).
func trimAncestors(src Source, branches []string) ([]string, error) {
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
			anc, err := src.IsAncestor(x, y)
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
