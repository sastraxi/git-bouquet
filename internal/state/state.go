// Package state persists in-progress rebuild state under .git/bouquet/.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName      = "bouquet"
	stateFile    = "state.json"
	worktreeName = "worktree"
)

type State struct {
	// Target is the branch to be updated at the end of the rebuild.
	Target string `json:"target"`
	// Base is the upstream branch the rebuild was started from.
	Base string `json:"base"`
	// BaseSHA is the resolved tip of Base when the rebuild started.
	BaseSHA string `json:"baseSha"`
	// PrevTargetSHA is the tip of Target when the rebuild started (empty if
	// the target branch did not yet exist).
	PrevTargetSHA string `json:"prevTargetSha,omitempty"`
	// Leaves is the ordered list of branches to merge.
	Leaves []string `json:"leaves"`
	// NextIndex is the index of the next leaf to merge. After the last leaf
	// is merged this equals len(Leaves) and we proceed to commit.
	NextIndex int `json:"nextIndex"`
	// LeafSHAs records the resolved SHA of each leaf at start time, for the
	// final commit message.
	LeafSHAs map[string]string `json:"leafShas"`
}

// Paths bundles the on-disk locations for state + worktree.
type Paths struct {
	Dir         string // .git/bouquet
	StateFile   string // .git/bouquet/state.json
	WorktreeDir string // .git/bouquet/worktree
}

// Locate returns the on-disk paths given the git common dir.
func Locate(gitCommonDir string) Paths {
	d := filepath.Join(gitCommonDir, dirName)
	return Paths{
		Dir:         d,
		StateFile:   filepath.Join(d, stateFile),
		WorktreeDir: filepath.Join(d, worktreeName),
	}
}

// Save writes the state to disk, creating the directory if needed.
func Save(p Paths, s *State) error {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", p.Dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.StateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.StateFile)
}

// Load reads state from disk. Returns (nil, nil) if no state exists.
func Load(p Paths) (*State, error) {
	data, err := os.ReadFile(p.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p.StateFile, err)
	}
	return &s, nil
}

// Clear removes the state file (but not the bouquet dir itself).
func Clear(p Paths) error {
	if err := os.Remove(p.StateFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
