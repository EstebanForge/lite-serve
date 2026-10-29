// Package home resolves the per-user state directory for lite-devenv.
// Layout under ~/.local/state/lite-devenv:
//
//	bin/        pinned frankenphp and mariadb binaries
//	run/        pidfiles, caddyfile, mariadb socket and datadir
//	log/        frankenphp.log, mariadb.log
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
	h, err := os.UserHomeDir()
	if err != nil {
		return Home{}, err
	}
	return Home{filepath.Join(h, ".local", "state", "lite-devenv")}, nil
}

func (h Home) Root() string  { return h.root }
func (h Home) Bin() string   { return filepath.Join(h.root, "bin") }
func (h Home) Run() string   { return filepath.Join(h.root, "run") }
func (h Home) Log() string   { return filepath.Join(h.root, "log") }
func (h Home) FrankenPHP() string {
	return filepath.Join(h.Bin(), "frankenphp")
}

func (h Home) MkdirAll() error {
	for _, d := range []string{h.Bin(), h.Run(), h.Log()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
