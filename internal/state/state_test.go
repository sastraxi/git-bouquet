package state

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLocate(t *testing.T) {
	p := Locate("/repo/.git")
	if p.Dir != "/repo/.git/bouquet" {
		t.Errorf("Dir: %s", p.Dir)
	}
	if p.StateFile != "/repo/.git/bouquet/state.json" {
		t.Errorf("StateFile: %s", p.StateFile)
	}
	if p.WorktreeDir != "/repo/.git/bouquet/worktree" {
		t.Errorf("WorktreeDir: %s", p.WorktreeDir)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := Locate(t.TempDir())
	want := &State{
		Target:        "release/current",
		Base:          "main",
		BaseSHA:       "deadbeef",
		PrevTargetSHA: "cafef00d",
		Leaves:        []string{"feat/a", "feat/b"},
		NextIndex:     1,
		LeafSHAs:      map[string]string{"feat/a": "111", "feat/b": "222"},
	}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadMissingReturnsNil(t *testing.T) {
	p := Locate(t.TempDir())
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for missing state, got %+v", got)
	}
}

func TestClearRemovesStateFile(t *testing.T) {
	p := Locate(t.TempDir())
	if err := Save(p, &State{Target: "t", Base: "b", Leaves: []string{}, LeafSHAs: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if err := Clear(p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil after clear, got %+v", got)
	}
	// Clear must also be idempotent.
	if err := Clear(p); err != nil {
		t.Errorf("second Clear failed: %v", err)
	}
}

func TestLoadCorrupt(t *testing.T) {
	p := Locate(t.TempDir())
	// Write garbage directly.
	if err := saveRaw(p, []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Error("expected error parsing corrupt state, got nil")
	}
}

// saveRaw is a test-only helper to write arbitrary bytes to the state file.
func saveRaw(p Paths, b []byte) error {
	if err := mkdirAll(p.Dir); err != nil {
		return err
	}
	return writeFile(filepath.Join(p.Dir, "state.json"), b)
}
