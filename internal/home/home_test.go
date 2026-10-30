package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathDoesNotMigrateLegacyState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("DEVENV_HOME", "")
	legacy := filepath.Join(root, ".local", "state", "lite-devenv")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Path(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy state changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".local", "state", "lite-serve")); !os.IsNotExist(err) {
		t.Fatal("Path migrated legacy state")
	}
}
