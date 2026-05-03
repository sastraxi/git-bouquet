package workflow

import "errors"

// errExitConflict is returned when the workflow stops because of an unresolved
// merge conflict. main.go translates this to exit code 2.
var errExitConflict = errors.New("merge conflict; resolve and run `git bouquet continue`")

// IsConflict reports whether err is a stop-for-conflict signal.
func IsConflict(err error) bool { return errors.Is(err, errExitConflict) }
