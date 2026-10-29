// devenv is the lite-devenv entrypoint: parked folders under ./sites
// served as https://<name>.localhost by frankenphp, no Docker.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/EstebanForge/lite-devenv/internal/bin"
	"github.com/EstebanForge/lite-devenv/internal/caddy"
	"github.com/EstebanForge/lite-devenv/internal/db"
	"github.com/EstebanForge/lite-devenv/internal/home"
	"github.com/EstebanForge/lite-devenv/internal/proc"
	"github.com/EstebanForge/lite-devenv/internal/site"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: devenv <start|stop|restart|status|sites>")
	fmt.Fprintln(os.Stderr, "       devenv db <client|import <dump.sql>|dump [file]|dsn>")
}

// projectDB is the database name for the project: its directory name,
// sanitized to MySQL identifier charset.
func projectDB(projectDir string) string {
	base := filepath.Base(projectDir)
	var b strings.Builder
	for _, r := range base {
		if r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	if name := b.String(); name != "" {
		return name
	}
	return "devenv"
}

func newDB(h home.Home) *db.DB {
	d := db.New(h.Root(), h.Run(), h.Log(), portEnvInt("DEVENV_DB_PORT", 3306))
	return &d
}

func run(cmd string) error {
	h, err := home.Dir()
	if err != nil {
		return err
	}
	if err := h.MkdirAll(); err != nil {
		return err
	}

	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}
	database := projectDB(projectDir)
	d := newDB(h)
	sites, serr := site.Discover(projectDir)

	if cmd == "db" {
		return runDB(d, database, os.Args[2:])
	}

	switch cmd {
	case "start", "restart":
		if serr != nil {
			return serr
		}
		if cmd == "restart" {
			_ = proc.Stop(h.Run(), h.FrankenPHP())
		}
		return start(h, database, sites, d)
	case "stop":
		_ = proc.Stop(h.Run(), h.FrankenPHP())
		if err := d.Stop(); err != nil {
			return err
		}
		fmt.Println("stopped")
		return nil
	case "status":
		pid, err := proc.PID(h.Run(), h.FrankenPHP())
		if err != nil {
			fmt.Println("frankenphp: not running")
		} else {
			fmt.Printf("frankenphp: running (pid %d, https port %s)\n", pid, portEnv("DEVENV_HTTPS_PORT", "443"))
		}
		if d.Running() {
			fmt.Printf("mysql:    running (127.0.0.1:%d, db %s)\n", d.Port, database)
		} else {
			fmt.Println("mysql:    not running")
		}
		if serr != nil {
			fmt.Println("sites:", serr)
		} else {
			for _, s := range sites {
				kind := "php"
				if s.WordPress {
					kind = "wordpress"
				}
				fmt.Printf("site: %-20s -> %s (%s)\n", "https://"+s.Host(), s.DocRoot, kind)
			}
		}
		return nil
	case "sites":
		if serr != nil {
			return serr
		}
		for _, s := range sites {
			fmt.Println(s.Name)
		}
		return nil
	}
	usage()
	return fmt.Errorf("unknown command %q", cmd)
}

// runDB handles `devenv db ...` against the project database.
func runDB(d *db.DB, database string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: devenv db <client|import <dump.sql>|dump [file]|dsn>")
	}
	switch args[0] {
	case "client":
		extra := []string{}
		if len(args) > 1 {
			extra = args[1:]
		}
		return d.Exec(database, extra)
	case "import":
		if len(args) != 2 {
			return fmt.Errorf("usage: devenv db import <dump.sql>")
		}
		return d.Import(database, args[1])
	case "dump":
		file := "-"
		if len(args) == 2 {
			file = args[1]
		} else if len(args) > 2 {
			return fmt.Errorf("usage: devenv db dump [file]")
		}
		return d.Dump(database, file)
	case "dsn":
		fmt.Println(d.DSN(database))
		return nil
	}
	return fmt.Errorf("unknown db subcommand %q", args[0])
}

func start(h home.Home, database string, sites []*site.Site, d *db.DB) error {
	franken, err := bin.FrankenPHP(h.Bin())
	if err != nil {
		return err
	}
	cfg, err := caddy.Render(sites, portEnvInt("DEVENV_HTTP_PORT", 80), portEnvInt("DEVENV_HTTPS_PORT", 443))
	if err != nil {
		return err
	}
	if err := d.EnsureInstalled(); err != nil {
		return err
	}
	if err := d.Start(); err != nil {
		return err
	}
	if err := d.EnsureDatabase(database); err != nil {
		return err
	}
	fmt.Printf("mysql: db %s at %s\n", database, d.DSN(database))

	caddyfile := filepath.Join(h.Run(), "Caddyfile")
	if err := os.WriteFile(caddyfile, []byte(cfg), 0o644); err != nil {
		return err
	}
	if err := proc.Start(franken, h.Run(), filepath.Join(h.Log(), "frankenphp.log"), caddyfile, portEnvInt("DEVENV_HTTPS_PORT", 443)); err != nil {
		return err
	}
	fmt.Println("serving:")
	for _, s := range sites {
		fmt.Printf("  https://%s\n", s.Host())
	}
	return nil
}

func portEnvInt(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func portEnv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
