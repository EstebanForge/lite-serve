package site

import (
	"os"
	"path/filepath"
	"testing"
)

func mk(t *testing.T, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDiscoverPublicDocroot(t *testing.T) {
	root := mk(t, "sites/demo/public")
	if err := os.WriteFile(filepath.Join(root, "sites/demo/public/index.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	sites, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 {
		t.Fatalf("want 1 site, got %d", len(sites))
	}
	s := sites[0]
	if s.Host() != "demo.localhost" {
		t.Errorf("host: got %s", s.Host())
	}
	if s.DocRoot != filepath.Join(root, "sites/demo/public") {
		t.Errorf("docroot: got %s", s.DocRoot)
	}
	if s.WordPress {
		t.Error("plain php dir flagged as wordpress")
	}
}

func TestDiscoverWordPressDetection(t *testing.T) {
	root := mk(t, "sites/blog")
	if err := os.WriteFile(filepath.Join(root, "sites/blog/wp-load.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	sites, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if !sites[0].WordPress {
		t.Error("wp-load.php present but site not flagged wordpress")
	}
}

func TestDiscoverSortedAndSkipsFiles(t *testing.T) {
	root := mk(t, "sites/bbb/public", "sites/aaa/public", "sites/README.md")
	// README.md is a file, not a dir: mk makes it as dir; fix by writing a real file
	os.RemoveAll(filepath.Join(root, "sites/README.md"))
	if err := os.WriteFile(filepath.Join(root, "sites/notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sites, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 || sites[0].Name != "aaa" || sites[1].Name != "bbb" {
		t.Fatalf("want [aaa bbb], got %+v", sites)
	}
}

func TestDiscoverSkipsHiddenAndFollowsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sites/real/public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sites/.hidden/public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "sites/real"), filepath.Join(root, "sites/linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	sites, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range sites {
		names = append(names, s.Name)
	}
	// .hidden has no docroot and must not fail discovery; linked must
	// resolve through the symlink.
	if len(sites) != 2 || names[0] != "linked" || names[1] != "real" {
		t.Fatalf("want [linked real], got %v", names)
	}
}

func TestDiscoverRejectsBadNames(t *testing.T) {
	root := mk(t, "sites/bad name/public", "sites/ok/public")
	if _, err := Discover(root); err == nil {
		t.Fatal("space in site name: want error")
	}
}

func TestDiscoverErrors(t *testing.T) {
	root := mk(t, "sites/empty")
	if _, err := Discover(root); err == nil {
		t.Error("empty sites dir: want error")
	}
}

func TestDiscoverSelf(t *testing.T) {
	root := t.TempDir()
	// the temp dir basename is not a valid host label (digits), so use a
	// named subdir like a real project folder
	proj := filepath.Join(root, "testme")
	if err := os.MkdirAll(filepath.Join(proj, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	sites, err := Discover(proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].Name != "testme" || sites[0].Host() != "testme.localhost" {
		t.Fatalf("want single site testme.localhost, got %+v", sites)
	}
	if sites[0].DocRoot != filepath.Join(proj, "public") {
		t.Errorf("docroot: want public/, got %s", sites[0].DocRoot)
	}
}

func TestDiscoverSelfRejectsBadNames(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bad name"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(filepath.Join(root, "bad name")); err == nil {
		t.Error("invalid basename: want error")
	}
}
