// Package home resolves the per-user state directory for lite-serve.
// Layout under ~/.local/state/lite-serve:
//
//	bin/         pinned frankenphp, mailpit and mysql binaries
//	run/         pidfiles, caddyfile, mysql socket
//	log/         frankenphp.log, mysqld.log, mailpit.log
//	registry.json  projects currently served by the shared daemon
package home

import (
	"os"
	"path/filepath"
)

type Home struct {
	root string
}

func Dir() (Home, error) {
	if v := os.Getenv("DEVENV_HOME"); v != "" {
		return Home{v}, nil
	}
	migrate()
	h, err := os.UserHomeDir()
	if err != nil {
		return Home{}, err
	}
	return Home{filepath.Join(h, ".local", "state", "lite-serve")}, nil
}

// migrate moves a pre-rename state dir (lite-devenv) into place once,
// so existing binaries and the MySQL datadir are not stranded.
func migrate() {
	h, err := os.UserHomeDir()
	if err != nil {
		return
	}
	old := filepath.Join(h, ".local", "state", "lite-devenv")
	if info, err := os.Stat(old); err != nil || !info.IsDir() {
		return
	}
	cur := filepath.Join(h, ".local", "state", "lite-serve")
	if _, err := os.Stat(cur); err == nil {
		return
	}
	_ = os.Rename(old, cur)
}

func (h Home) Root() string { return h.root }
func (h Home) Bin() string  { return filepath.Join(h.root, "bin") }
func (h Home) Run() string  { return filepath.Join(h.root, "run") }
func (h Home) Log() string  { return filepath.Join(h.root, "log") }
func (h Home) FrankenPHP() string {
	return filepath.Join(h.Bin(), "frankenphp")
}

func (h Home) FrankenPHPpid() string {
	return filepath.Join(h.Run(), "frankenphp.pid")
}

func (h Home) Caddyfile() string {
	return filepath.Join(h.Run(), "Caddyfile")
}

func (h Home) SessionWatchPid() string {
	return filepath.Join(h.Run(), "session-watch.pid")
}

func (h Home) SessionID() string {
	return filepath.Join(h.Run(), "session.id")
}

func (h Home) MkdirAll() error {
	for _, d := range []string{h.Bin(), h.Run(), h.Log()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
