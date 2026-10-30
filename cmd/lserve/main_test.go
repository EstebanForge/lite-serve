package main

import "testing"

func TestParseUpProxyFlags(t *testing.T) {
	opts, err := parseUpFlags([]string{"/tmp/app", "--proxy", "3000", "--name", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.proxySet || opts.proxyPort != 3000 || opts.name != "api" || opts.dir != "/tmp/app" {
		t.Fatalf("unexpected options: %+v", opts)
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
