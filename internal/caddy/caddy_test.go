package caddy

import (
	"strings"
	"testing"

	"github.com/EstebanForge/lite-devenv/internal/site"
)

func TestRenderBasicShape(t *testing.T) {
	sites := []*site.Site{{Name: "demo", DocRoot: "/srv/demo/public"}}
	out, err := Render(sites, 80, 8443)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https_port 8443",
		"https://demo.localhost:8443",
		"bind 127.0.0.1 ::1",
		"tls internal",
		`root * "/srv/demo/public"`,
		"php_server\n",                 // non-WP: bare directive, no env block
		"https://demo.localhost:443\"", // must not appear: port is 8443
	} {
		if want == "https://demo.localhost:443\"" {
			if strings.Contains(out, want) {
				t.Errorf("unexpected default port in:\n%s", out)
			}
			continue
		}
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "WP_HOME") {
		t.Errorf("non-WP site got WP env:\n%s", out)
	}
}

func TestRenderWordPressEnv(t *testing.T) {
	sites := []*site.Site{{Name: "blog", DocRoot: "/srv/blog", WordPress: true}}
	out, err := Render(sites, 80, 443)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "env WP_HOME https://blog.localhost") {
		t.Errorf("WP_HOME with default port omitted missing:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "env WP_HOME") && strings.Contains(line, ":443") {
			t.Errorf("WP_HOME must omit default port: %s", line)
		}
	}
}

func TestRenderErrors(t *testing.T) {
	if _, err := Render(nil, 80, 443); err == nil {
		t.Error("no sites: want error")
	}
	if _, err := Render([]*site.Site{{Name: "a"}}, 0, 443); err == nil {
		t.Error("bad port: want error")
	}
}
