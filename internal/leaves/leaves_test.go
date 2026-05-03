package leaves

import (
	"reflect"
	"testing"
)

// fakeSrc is an in-memory Source for tests. ancestors[a] is the set of
// branches a is an ancestor of (so a -> {b} means IsAncestor(a, b) == true).
type fakeSrc struct {
	branches  []string
	ancestors map[string]map[string]bool
}

func (f *fakeSrc) LocalBranches() ([]string, error) { return f.branches, nil }

func (f *fakeSrc) IsAncestor(a, b string) (bool, error) {
	return f.ancestors[a][b], nil
}

func newFake(branches ...string) *fakeSrc {
	return &fakeSrc{branches: branches, ancestors: map[string]map[string]bool{}}
}

// link records a -> b: a is an ancestor of b.
func (f *fakeSrc) link(a, b string) *fakeSrc {
	if f.ancestors[a] == nil {
		f.ancestors[a] = map[string]bool{}
	}
	f.ancestors[a][b] = true
	return f
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name     string
		src      *fakeSrc
		patterns []string
		excludes []string
		want     []string
	}{
		{
			name:     "single glob expands",
			src:      newFake("feat/a", "feat/b", "main", "test/x"),
			patterns: []string{"feat/*"},
			want:     []string{"feat/a", "feat/b"},
		},
		{
			name:     "star crosses slashes (git refspec semantics)",
			src:      newFake("feat/a", "feat/a/b", "feat/a/b/c"),
			patterns: []string{"feat/*"},
			want:     []string{"feat/a", "feat/a/b", "feat/a/b/c"},
		},
		{
			name:     "multiple includes union and dedupe",
			src:      newFake("feat/a", "test/x", "fix/y"),
			patterns: []string{"feat/*", "test/*", "feat/*"},
			want:     []string{"feat/a", "test/x"},
		},
		{
			name:     "glob expansion preserves git branch order",
			src:      newFake("feat/zebra", "feat/apple", "feat/mango"),
			patterns: []string{"feat/*"},
			want:     []string{"feat/zebra", "feat/apple", "feat/mango"},
		},
		{
			name: "explicit entry after negation appends to end",
			src:  newFake("feat/bouquet", "feat/apple", "feat/mango"),
			patterns: []string{"feat/*", "!feat/bouquet", "feat/bouquet"},
			want: []string{"feat/apple", "feat/mango", "feat/bouquet"},
		},
		{
			name:     "positional excludes drop matches",
			src:      newFake("feat/a", "feat/b", "main"),
			patterns: []string{"feat/*"},
			excludes: []string{"feat/b"},
			want:     []string{"feat/a"},
		},
		{
			name:     "negative pattern drops matches",
			src:      newFake("feat/a", "feat/wip-x", "feat/wip-y"),
			patterns: []string{"feat/*", "!feat/wip-*"},
			want:     []string{"feat/a"},
		},
		{
			name:     "negative pattern overrides include even if include is more specific",
			src:      newFake("feat/clip-indicator", "feat/other"),
			patterns: []string{"feat/clip-indicator", "feat/other", "!feat/clip-indicator"},
			want:     []string{"feat/other"},
		},
		{
			name:     "no matches returns empty",
			src:      newFake("main", "develop"),
			patterns: []string{"feat/*"},
			want:     []string{},
		},
		{
			name: "ancestor trim drops parent when child also selected",
			src: newFake("feat/parent", "feat/child", "feat/unrelated").
				link("feat/parent", "feat/child"),
			patterns: []string{"feat/*"},
			want:     []string{"feat/child", "feat/unrelated"},
		},
		{
			name: "ancestor trim handles chains",
			src: newFake("a", "b", "c").
				link("a", "b").
				link("b", "c").
				link("a", "c"),
			patterns: []string{"*"},
			want:     []string{"c"},
		},
		{
			name: "ancestor trim respects independent branches",
			src: newFake("feat/a", "feat/b").
				link("base", "feat/a").
				link("base", "feat/b"),
			patterns: []string{"feat/*"},
			want:     []string{"feat/a", "feat/b"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.src, tc.patterns, tc.excludes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveInvalidGlob(t *testing.T) {
	src := newFake("feat/a")
	_, err := Resolve(src, []string{"feat/["}, nil)
	if err == nil {
		t.Fatal("expected error for malformed glob, got nil")
	}
}
