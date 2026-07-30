package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

// ShortUnixSocketPath creates a pathname-backed Unix socket location that does
// not inherit a potentially deep TMPDIR. Linux sockaddr_un paths are bounded,
// while Codex and some CI runners use deeply nested temporary directories.
func ShortUnixSocketPath(t testing.TB, name string) string {
	t.Helper()
	if name == "" || filepath.Base(name) != name {
		t.Fatalf("unix socket name must be one non-empty path element: %q", name)
	}
	directory, err := os.MkdirTemp("/tmp", "praxis-sandbox-socket-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove unix socket test directory: %v", err)
		}
	})
	return filepath.Join(directory, name)
}
