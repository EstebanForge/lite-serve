// Package caddy renders the shared frankenphp Caddyfile: every site of
// every registered project becomes an https://<name>.localhost block.
// One daemon serves all projects on one port, valet-style.
package caddy

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/EstebanForge/lite-serve/internal/site"
)

const caddyfileTemplate = `
{
	http_port {{.HTTPPort}}
	https_port {{.HTTPSPort}}
	auto_https disable_redirects
}

{{range .Sites -}}
https://{{.Host}}:{{$.HTTPSPort}} {
	bind 127.0.0.1 ::1
	tls internal
{{if eq .Mode "proxy" -}}
	reverse_proxy {{.Target}}
	handle_errors {
		@bad_gateway expression {http.error.status_code} == 502
		respond @bad_gateway "Nothing is listening on {{.Target}}. Start your app: {{.StartHint}}" 502
		respond "Proxy error: {http.error.status_code}" {http.error.status_code}
	}
{{else -}}
	root * "{{.DocRoot}}"
	encode zstd gzip
{{if .WordPress -}}
	# WP_HOME/WP_SITEURL injected inside php_server: frankenphp worker
	# mode does not reliably inherit parent env.
	php_server {
		env WP_HOME {{siteURL $.HTTPSPort .Host}}
		env WP_SITEURL {{siteURL $.HTTPSPort .Host}}
	}
{{else -}}
	php_server
{{end -}}
{{end -}}
}
{{end}}
`

type templateData struct {
	Sites     []*site.Site
	HTTPPort  int
	HTTPSPort int
}

var tmpl = template.Must(template.New("caddyfile").Funcs(template.FuncMap{
	"siteURL": siteURL,
}).Parse(caddyfileTemplate))

// siteURL builds the public site URL, omitting the port when it is the
// https default: WP_HOME with an explicit :443 makes WordPress
// redirect_canonical loop.
func siteURL(httpsPort int, host string) string {
	if httpsPort == 443 {
		return "https://" + host
	}
	return "https://" + host + ":" + strconv.Itoa(httpsPort)
}

// Render builds the Caddyfile for all projects' sites. Hostnames must
// be unique across projects: Caddy refuses duplicate site addresses, so
// the collision is reported here with both project paths.
func Render(projects map[string][]*site.Site, httpPort, httpsPort int) (string, error) {
	if len(projects) == 0 {
		return "", fmt.Errorf("no projects registered")
	}
	if httpPort <= 0 || httpPort > 65535 || httpsPort <= 0 || httpsPort > 65535 {
		return "", fmt.Errorf("invalid ports: http=%d https=%d", httpPort, httpsPort)
	}
	owners := map[string]string{}
	data := templateData{HTTPPort: httpPort, HTTPSPort: httpsPort}
	for dir, sites := range projects {
		for _, s := range sites {
			// DNS is case-insensitive: Demo and demo are one hostname
			// and Caddy would reject the duplicate block.
			key := strings.ToLower(s.Host())
			if owner, taken := owners[key]; taken {
				return "", fmt.Errorf("site name collision: %q is served by both %s and %s; rename one folder", s.Host(), owner, dir)
			}
			owners[key] = dir
			data.Sites = append(data.Sites, s)
		}
	}
	// Map iteration order is random; stable output keeps reload diffs
	// readable and behavior reproducible.
	sort.Slice(data.Sites, func(i, j int) bool {
		return data.Sites[i].Host() < data.Sites[j].Host()
	})
	if len(data.Sites) == 0 {
		return "", fmt.Errorf("no sites across registered projects")
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimLeft(buf.String(), "\n"), nil
}
