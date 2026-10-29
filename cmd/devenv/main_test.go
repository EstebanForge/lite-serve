package main

import "testing"

func TestProjectDBKeepsHyphens(t *testing.T) {
	if got := projectDB("/home/x/my-site"); got != "my-site" {
		t.Errorf("my-site: got %q", got)
	}
	if got := projectDB("/home/x/mysite"); got != "mysite" {
		t.Errorf("mysite: got %q", got)
	}
	if got := projectDB("/home/x/my site!"); got != "mysite" {
		t.Errorf("sanitized: got %q", got)
	}
	if got := projectDB("/home/x/网站"); got != "devenv" {
		t.Errorf("non-ascii fallback: got %q", got)
	}
}
