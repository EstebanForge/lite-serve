// Package caddy renders the frankenphp (Caddy) config: one site block
// per parked site, loopback bind, internal CA TLS. WordPress sites get
// WP_HOME/WP_SITEURL env inside php_server because Caddy rejects
// site-level env and WordPress compares the injected URL against
// HTTP_HOST.
package caddy

import (
	"bytes"
	"fmt"
	"sort"
	"text/template"

	"github.com/EstebanForge/lite-devenv/internal/site"
)

var tmpl = template.Must(template.New("caddyfile").Funcs(template.FuncMap{
	"siteURL": siteURL,
}).Parse(`{
	admin off
	http_port {{.HTTPPort}}
	https_port {{.HTTPSPort}}
}
{{range .Sites}}
https://{{.Host}}:{{$.HTTPSPort}} {
	bind 127.0.0.1 ::1
	tls internal
	root * "{{.DocRoot}}"
	{{if .WordPress}}php_server {
		env WP_HOME {{siteURL .Host $.HTTPSPort}}
		env WP_SITEURL {{siteURL .Host $.HTTPSPort}}
	}
	{{else}}php_server
	{{end}}
}
{{end}}`))

func siteURL(host string, httpsPort int) string {
	if httpsPort == 443 {
		return "https://" + host
	}
	return fmt.Sprintf("https://%s:%d", host, httpsPort)
}

// Render returns the Caddyfile body for the sites. Empty site list is
// an error: an empty config would make frankenphp exit immediately.
func Render(sites []*site.Site, httpPort, httpsPort int) (string, error) {
	if len(sites) == 0 {
		return "", fmt.Errorf("no sites to render")
	}
	if httpPort <= 0 || httpsPort <= 0 {
		return "", fmt.Errorf("ports must be positive: http=%d https=%d", httpPort, httpsPort)
	}
	ordered := make([]*site.Site, len(sites))
	copy(ordered, sites)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })

	var buf bytes.Buffer
	data := struct {
		Sites     []*site.Site
		HTTPPort  int
		HTTPSPort int
	}{ordered, httpPort, httpsPort}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
