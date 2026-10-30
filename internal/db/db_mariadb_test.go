package db

import (
	"os"
	"path/filepath"

	"testing"
)

// TestDetectFlavor pins the version-string parsing both ways.
func TestDetectFlavor(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "mysqld")
	script := "#!/bin/sh\necho '" + `mysqld  Ver 11.8.9-MariaDB for osx10.19 on arm64 (Homebrew)` + "'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// PATH lookup needs the exact name
	t.Setenv("PATH", tmp+string(filepath.ListSeparator)+os.Getenv("PATH"))
	d := DB{Base: ""}
	if err := d.detectFlavor(); err != nil {
		t.Fatal(err)
	}
	if d.flavor != "mariadb" {
		t.Errorf("want mariadb, got %q", d.flavor)
	}
}
