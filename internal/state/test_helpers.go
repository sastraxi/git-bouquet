package state

import "os"

// Tiny helpers exposed for the test file in this package.
func mkdirAll(dir string) error            { return os.MkdirAll(dir, 0o755) }
func writeFile(path string, b []byte) error { return os.WriteFile(path, b, 0o644) }
