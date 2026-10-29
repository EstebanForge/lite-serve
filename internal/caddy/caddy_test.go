package caddy

import (
	"strings"
	"testing"

	"github.com/EstebanForge/lite-devenv/internal/site"
)

func TestRenderBasicShape(t *testing.T) {
	projects := map[string][]*site.Site{
		"/srv/demo": {{Name: "demo", DocRoot: "/srv/demo/public"}},
	}
	out, err := Render(projects, 80, 8443)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https_port 8443",
		"https://demo.localhost:8443",
		"bind 127.0.0.1 ::1",
		"tls internal",
		`root * "/srv/demo/public"`,
		"php_server\n", // non-WP: bare directive, no env block
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "https://demo.localhost:443\"") {
		t.Errorf("unexpected default port in:\n%s", out)
	}
	if strings.Contains(out, "WP_HOME") {
		t.Errorf("non-WP site got WP env:\n%s", out)
	}
}

func TestRenderWordPressEnv(t *testing.T) {
	projects := map[string][]*site.Site{
		"/srv/blog": {{Name: "blog", DocRoot: "/srv/blog", WordPress: true}},
	}
	out, err := Render(projects, 80, 443)
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

func TestRenderCollision(t *testing.T) {
	projects := map[string][]*site.Site{
		"/a": {{Name: "demo", DocRoot: "/a/public"}},
		"/b": {{Name: "demo", DocRoot: "/b/public"}},
	}
	if _, err := Render(projects, 80, 443); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Errorf("want collision error, got %v", err)
	}
}

func TestRenderErrors(t *testing.T) {
	if _, err := Render(nil, 80, 443); err == nil {
		t.Error("no projects: want error")
	}
	if _, err := Render(map[string][]*site.Site{"/a": nil}, 0, 443); err == nil {
		t.Error("bad port: want error")
	}
}
