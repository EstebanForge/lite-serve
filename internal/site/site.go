// Package site discovers sites two ways. A ./sites directory parks
// multiple sites: every child becomes <dirname>.localhost. Without
// ./sites, the directory itself is served as one site named after its
// basename. Docroot resolves by first match of public/, web/, or the
// directory itself; wp-load.php marks a WordPress site for URL env
// injection.
package site

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Site struct {
	Name      string // directory name, becomes the subdomain
	Root      string // absolute path to the site directory
	DocRoot   string // absolute path to the document root
	WordPress bool
}

// Discover returns the sites of a project directory. With ./sites it
// parks one site per child directory; without it the directory itself
// is the single site, named by its basename.
func Discover(projectDir string) ([]*Site, error) {
	return DiscoverWithDocroot(projectDir, "")
}

// DiscoverWithDocroot is Discover for the single-site case with an
// explicit docroot override (the --dir flag): the site is still named
// after projectDir, but serves subdir instead. Overrides are rejected
// when ./sites exists, where folder names already decide docroots.
func DiscoverWithDocroot(projectDir, subdir string) ([]*Site, error) {
	sitesDir := filepath.Join(projectDir, "sites")
	if info, err := os.Stat(sitesDir); err != nil || !info.IsDir() {
		return discoverSelf(projectDir, subdir)
	}
	if subdir != "" {
		return nil, fmt.Errorf("--dir only applies when there is no ./sites directory")
	}
	entries, err := os.ReadDir(sitesDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", sitesDir, err)
	}
	var sites []*Site
	for _, e := range entries {
		// Hidden entries (.git, .DS_Store, editor droppings) are never
		// sites; skipping keeps one bad entry from failing all sites.
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// os.Stat follows symlinks, so linked-in project folders work.
		info, err := os.Stat(filepath.Join(sitesDir, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		if !validName(e.Name()) {
			return nil, fmt.Errorf("site %q: name must be a valid host label (letters, digits, hyphens; not starting or ending with a hyphen)", e.Name())
		}
		root := filepath.Join(sitesDir, e.Name())
		doc, err := docroot(root)
		if err != nil {
			return nil, fmt.Errorf("site %s: %w", e.Name(), err)
		}
		sites = append(sites, &Site{
			Name:      e.Name(),
			Root:      root,
			DocRoot:   doc,
			WordPress: fileExists(filepath.Join(doc, "wp-load.php")),
		})
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("no sites in %s", sitesDir)
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	return sites, nil
}

// discoverSelf serves projectDir itself as one site, named by its
// basename. Guards against the two directories that would leak the
// whole disk: / and the user's home.
func discoverSelf(projectDir, subdir string) ([]*Site, error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}
	if home, herr := os.UserHomeDir(); herr == nil && (abs == home || abs == filepath.Dir(home)) {
		return nil, fmt.Errorf("refusing to serve %s itself; run lserve up inside your project folder (or create a ./sites directory)", abs)
	}
	name := filepath.Base(abs)
	if !validName(name) {
		return nil, fmt.Errorf("directory name %q is not a valid host label; rename it or use a ./sites/<name> folder", name)
	}
	root := abs
	if subdir != "" {
		root = filepath.Join(abs, subdir)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("--dir %q: not a directory in %s", subdir, abs)
		}
	}
	doc, err := docroot(root)
	if err != nil {
		return nil, fmt.Errorf("site %s: %w", name, err)
	}
	return []*Site{{
		Name:      name,
		Root:      abs,
		DocRoot:   doc,
		WordPress: fileExists(filepath.Join(doc, "wp-load.php")),
	}}, nil
}

// Host is the served hostname for the site.
func (s *Site) Host() string { return s.Name + ".localhost" }

// validName enforces an RFC 1123 host label: directory names become
// subdomains, so anything else breaks TLS SNI or the Caddyfile parse.
func validName(name string) bool {
	if len(name) == 0 || len(name) > 63 || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for _, r := range name {
		if r != '-' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func docroot(root string) (string, error) {
	for _, d := range []string{"public", "web"} {
		candidate := filepath.Join(root, d)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	// No conventional subdir: a WordPress checkout or any index file at
	// the root also makes it a docroot.
	if fileExists(filepath.Join(root, "wp-load.php")) {
		return root, nil
	}
	for _, f := range []string{"index.php", "index.html"} {
		if fileExists(filepath.Join(root, f)) {
			return root, nil
		}
	}
	return "", fmt.Errorf("no docroot: add %s or an index.php", strings.Join([]string{"public/", "web/"}, " or "))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
