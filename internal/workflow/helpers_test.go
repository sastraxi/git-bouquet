package workflow

import (
	"strings"
	"testing"

	"github.com/sastraxi/git-bouquet/internal/state"
)

func TestShortSHA(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "abc"},
		{"abcdefghijkl", "abcdefghijkl"},
		{"abcdefghijklmnop", "abcdefghijkl"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := shortSHA(tc.in); got != tc.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildCommitMessage(t *testing.T) {
	st := &state.State{
		Target:  "release/current",
		Base:    "main",
		BaseSHA: "1111111111112222",
		Leaves:  []string{"feat/a", "feat/b"},
		LeafSHAs: map[string]string{
			"feat/a": "aaaaaaaaaaaabbbb",
			"feat/b": "ccccccccccccdddd",
		},
	}
	msg := buildCommitMessage(st)
	for _, want := range []string{
		"bouquet: rebuild ",
		"from main + 2 leaves",
		"base: main @ 111111111111",
		"feat/a @ aaaaaaaaaaaa",
		"feat/b @ cccccccccccc",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}
