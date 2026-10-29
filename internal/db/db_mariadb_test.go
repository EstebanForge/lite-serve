package db

import (
	"os"
	"os/exec"
	"path/filepath"

	"testing"
)

// TestSystemMariaDB exercises the PATH-based install path (macOS, or
// any machine with MariaDB on PATH) against a throwaway datadir.
// Skips where no server binaries exist, so CI's MySQL-tarball path is
// untouched.
func TestSystemMariaDB(t *testing.T) {
	if testing.Short() {
		t.Skip("spins up a real server")
	}
	if _, err := exec.LookPath("mysqld"); err != nil {
		t.Skip("no mysqld on PATH")
	}
	if _, err := exec.LookPath("mariadb-install-db"); err != nil {
		if _, err := exec.LookPath("mysql_install_db"); err != nil {
			t.Skip("no mariadb-install-db on PATH")
		}
	}
	tmp := t.TempDir()
	d := DB{
		Base:   "", // resolve from PATH
		Run:    filepath.Join(tmp, "run"),
		Data:   filepath.Join(tmp, "data"),
		LogDir: tmp,
		Port:   3310,
	}
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Stop() })
	if err := d.EnsureDatabase("lite_serve_test"); err != nil {
		t.Fatal(err)
	}
	out, err := d.exec("SELECT 1")
	if err != nil {
		t.Fatalf("query over TCP as root: %v: %s", err, out)
	}
}

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
