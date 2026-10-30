package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EstebanForge/lite-serve/internal/site"
)

func TestParseUpProxyFlags(t *testing.T) {
	opts, err := parseUpFlags([]string{"/tmp/app", "--proxy", "3000", "--name", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.proxySet || opts.proxyPort != 3000 || opts.name != "api" || opts.dir != "/tmp/app" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestConfigureProjectDoesNotWriteBeforeValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sites"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := upOptions{dir: dir, proxyPort: 3000, proxySet: true}
	cfg, err := configureProject(opts, strings.NewReader(""), io.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.Mode != site.ModeProxy {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, site.ConfigFile)); !os.IsNotExist(err) {
		t.Fatal("configureProject persisted config")
	}
	if _, err := site.DiscoverWithConfig(dir, "", *cfg); err == nil {
		t.Fatal("invalid proxy/sites combination passed discovery")
	}
}

func TestConfigureProjectPromptValidation(t *testing.T) {
	for _, input := range []string{"", "maybe\n", "y\nnot-a-port\n"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		opts := upOptions{dir: dir}
		if _, err := configureProject(opts, strings.NewReader(input), io.Discard, true); err == nil {
			t.Errorf("input %q was accepted", input)
		}
		if _, err := os.Stat(filepath.Join(dir, site.ConfigFile)); !os.IsNotExist(err) {
			t.Errorf("input %q left config on disk", input)
		}
	}
}

func TestConfigureProjectMixedPHPStaysDirect(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"package.json": "{}", "public/index.php": "<?php"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := configureProject(upOptions{dir: dir}, strings.NewReader(""), io.Discard, false)
	if err != nil || cfg != nil {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestParseUpRejectsConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"--proxy", "3000", "--dir", "public"},
		{"--name", "api"},
		{"--proxy", "not-a-port"},
		{"--unknown"},
	} {
		if _, err := parseUpFlags(args); err == nil {
			t.Errorf("args %v were accepted", args)
		}
	}
}
