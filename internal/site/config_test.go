package site

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigProxyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Config{Mode: ModeProxy, Port: 3000, Name: "api"}
	if err := SaveConfig(dir, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadConfig(dir)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestConfigRejectsBadPorts(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 8443, 3306} {
		cfg := Config{Mode: ModeProxy, Port: port}
		if err := cfg.Validate(); err == nil {
			t.Errorf("port %d was accepted", port)
		}
	}
}

func TestDiscoverProxy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(dir, Config{Mode: ModeProxy, Port: 3000}); err != nil {
		t.Fatal(err)
	}
	sites, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].Mode != ModeProxy || sites[0].Target != "localhost:3000" || sites[0].Kind != "node" {
		t.Fatalf("unexpected sites: %+v", sites)
	}
}

func TestDiscoverProxyRejectsSitesAndDocroot(t *testing.T) {
	dir := t.TempDir()
	if err := SaveConfig(dir, Config{Mode: ModeProxy, Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sites"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil {
		t.Fatal("proxy with sites/ was accepted")
	}
	if err := os.Remove(filepath.Join(dir, "sites")); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverWithDocroot(dir, "public"); err == nil {
		t.Fatal("proxy with --dir was accepted")
	}
}

func TestDetectPHPWinsOnMixedProject(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"composer.json", "package.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if kind, _, direct := Detect(dir); !direct || kind != "" {
		t.Fatalf("kind=%q direct=%v", kind, direct)
	}
}

func TestDetectDirectDocrootsBeforeAppMarkers(t *testing.T) {
	for _, file := range []string{"public/index.php", "web/index.php", "index.html", "public/index.html"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, direct := Detect(dir); !direct {
			t.Errorf("%s was not detected as directly servable", file)
		}
	}
}

func TestDetectWithDocroot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "admin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin", "index.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, direct := DetectWithDocroot(dir, "admin"); !direct {
		t.Fatal("--dir docroot was ignored")
	}
}

func TestGoDefaultPortIsValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, port, _ := Detect(dir)
	if err := (Config{Mode: ModeProxy, Port: port}).Validate(); err != nil {
		t.Fatalf("detected Go default %d is invalid: %v", port, err)
	}
}

func TestReservedPortsFollowEnvironment(t *testing.T) {
	t.Setenv("DEVENV_HTTPS_PORT", "9443")
	if err := (Config{Mode: ModeProxy, Port: 9443}).Validate(); err == nil {
		t.Fatal("configured HTTPS port was accepted as a proxy target")
	}
}

func TestDiscoverProxyRefusesHomeAndRoot(t *testing.T) {
	for _, dir := range []string{"/", os.Getenv("HOME")} {
		if _, err := DiscoverWithConfig(dir, "", Config{Mode: ModeProxy, Port: 3000}); err == nil {
			t.Errorf("proxy discovery accepted %s", dir)
		}
	}
}
