// Package leaves expands merge globs into an ancestor-trimmed leaf list.
package leaves

import (
	"fmt"

	"github.com/gobwas/glob"
)

// Source is the minimal git surface area Resolve needs. *git.Repo satisfies
// it implicitly; tests provide an in-memory fake.
type Source interface {
	LocalBranches() ([]string, error)
	IsAncestor(a, b string) (bool, error)
}

// Resolve expands the given glob patterns against local branches, drops
// branches matching `excludes`, and removes ancestors (if A is an ancestor of
// B and both are in the set, drop A).
//
// Patterns are processed left-to-right, preserving insertion order:
//   - A positive pattern appends every matching branch not yet in the list.
//   - A negative pattern (prefixed with `!`) removes all matching branches
//     from the list accumulated so far.
//
// This means the order of entries in .bouquet.yaml controls merge order.
// Within a single glob expansion the order follows LocalBranches() (typically
// alphabetical, but not guaranteed — callers that need strict determinism
// should use explicit branch names rather than globs).
//
// Glob semantics: gobwas/glob with no separator, so `*` crosses `/` to match
// git refspec conventions (e.g. `feat/*` matches `feat/a/b`).
func Resolve(src Source, patterns, excludes []string) ([]string, error) {
	branches, err := src.LocalBranches()
	if err != nil {
		return nil, fmt.Errorf("listing local branches: %w", err)
	}
	excludeSet := make(map[string]struct{}, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = struct{}{}
	}

	var list []string
	seen := make(map[string]struct{})

	for _, p := range patterns {
		neg := len(p) > 0 && p[0] == '!'
		raw := p
		if neg {
			raw = p[1:]
		}
		g, err := glob.Compile(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", p, err)
		}

		if neg {
			kept := list[:0:0]
			for _, b := range list {
				if g.Match(b) {
					delete(seen, b)
				} else {
					kept = append(kept, b)
				}
			}
			list = kept
		} else {
			for _, b := range branches {
				if _, skip := excludeSet[b]; skip {
					continue
				}
				if _, already := seen[b]; already {
					continue
				}
				if g.Match(b) {
					list = append(list, b)
					seen[b] = struct{}{}
				}
			}
		}
	}

	trimmed, err := trimAncestors(src, list)
	if err != nil {
		return nil, err
	}
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
