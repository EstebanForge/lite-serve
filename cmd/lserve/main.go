// lserve is the lite-serve entrypoint: one shared frankenphp daemon
// serves every registered project's ./sites folders as
// https://<name>.localhost. Daemons detach, so the terminal is never
// locked; `lserve list` manages what is running.
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/EstebanForge/lite-serve/internal/bin"
	"github.com/EstebanForge/lite-serve/internal/caddy"
	"github.com/EstebanForge/lite-serve/internal/db"
	"github.com/EstebanForge/lite-serve/internal/home"
	"github.com/EstebanForge/lite-serve/internal/mailpit"
	"github.com/EstebanForge/lite-serve/internal/proc"
	"github.com/EstebanForge/lite-serve/internal/registry"
	"github.com/EstebanForge/lite-serve/internal/site"
	"github.com/EstebanForge/lite-serve/internal/tui"
)

type app struct {
	h       home.Home
	reg     *registry.Registry
	franken string
	db      *db.DB
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	if err := run(args[0], args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: lserve <command>

  up        serve ./sites of the current directory (background daemon)
  down      stop serving this directory (or: down <dir>)
  restart   re-discover sites and reload (or: restart <dir>)
  list      running projects: interactive picker, or a table when piped
  logs      tail the frankenphp log (or: logs mailpit)
  mailpit   mailpit menu: start|stop|restart|status (interactive when a TTY)
  db        client|import <dump.sql>|dump [file]|dsn

ports: DEVENV_HTTP_PORT=8080 DEVENV_HTTPS_PORT=8443 DEVENV_DB_PORT=3306
       DEVENV_MAILPIT_UI_PORT=8025 DEVENV_MAILPIT_SMTP_PORT=1025`)
}

func newApp() (*app, error) {
	h, err := home.Dir()
	if err != nil {
		return nil, err
	}
	if err := h.MkdirAll(); err != nil {
		return nil, err
	}
	reg, err := registry.Open(h.Root())
	if err != nil {
		return nil, err
	}
	d := db.New(h.Root(), h.Run(), h.Log(), portEnvInt("DEVENV_DB_PORT", 3306))
	franken, err := bin.FrankenPHP(h.Bin())
	if err != nil {
		return nil, err
	}
	return &app{h: h, reg: reg, franken: franken, db: &d}, nil
}

func run(cmd string, args []string) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	switch cmd {
	case "up":
		return a.up(mustWd())
	case "down":
		return a.down(targetDir(args))
	case "restart":
		return a.restart(targetDir(args))
	case "list":
		return a.list()
	case "logs":
		return a.logs(args)
	case "mailpit":
		return a.mailpitCmd(args)
	case "db":
		return a.runDB(projectDB(mustWd()), args)
	}
	usage()
	return fmt.Errorf("unknown command %q", cmd)
}

func mustWd() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	return wd
}

// targetDir resolves an optional dir argument, defaulting to cwd.
func targetDir(args []string) string {
	if len(args) > 0 {
		dir, err := filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return dir
	}
	return mustWd()
}

// discoverAll maps every registered project to its sites. An
// unreadable project is skipped with a warning, not fatal: one broken
// folder must not wedge every other project's reload.
func (a *app) discoverAll() (map[string][]*site.Site, error) {
	projects := map[string][]*site.Site{}
	for _, dir := range a.reg.Sorted() {
		sites, err := site.Discover(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable project %s: %v\n", dir, err)
			continue
		}
		projects[dir] = sites
	}
	return projects, nil
}

// reload re-renders the Caddyfile and restarts the daemon when the
// registry is non-empty, or stops everything when it is empty.
func (a *app) reload() error {
	if len(a.reg.Projects) == 0 {
		_ = proc.Stop(a.h.FrankenPHPpid(), a.franken)
		return a.db.Stop()
	}
	projects, err := a.discoverAll()
	if err != nil {
		return err
	}
	cfg, err := caddy.Render(projects, portEnvInt("DEVENV_HTTP_PORT", 8080), portEnvInt("DEVENV_HTTPS_PORT", 8443))
	if err != nil {
		return err
	}
	// Atomic write: an in-place truncate would leave a 0-byte config if
	// two reloads race or the write is interrupted.
	tmp := fmt.Sprintf("%s.%d.tmp", a.h.Caddyfile(), os.Getpid())
	if err := os.WriteFile(tmp, []byte(cfg), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, a.h.Caddyfile()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if _, err := proc.PID(a.h.FrankenPHPpid(), a.franken); err == nil {
		// Live daemon: graceful reload keeps connections for every
		// other project. Falls back to restart if the reload fails.
		cmd := exec.Command(a.franken, "reload", "--config", a.h.Caddyfile(), "--adapter", "caddyfile")
		if _, err := cmd.CombinedOutput(); err == nil {
			return a.waitDaemonUp()
		}
	}
	_ = proc.Stop(a.h.FrankenPHPpid(), a.franken)
	if err := proc.Start(a.franken, []string{"run", "--config", a.h.Caddyfile(), "--adapter", "caddyfile"}, a.h.FrankenPHPpid(), filepath.Join(a.h.Log(), "frankenphp.log")); err != nil {
		return err
	}
	return a.waitDaemonUp()
}

// waitDaemonUp requires BOTH our daemon process alive AND its port
// answering. Port alone is not proof: any squatter (the macOS tunnel,
// another service) answers too, and a bind failure then looks healthy.
func (a *app) waitDaemonUp() error {
	port := portEnvInt("DEVENV_HTTPS_PORT", 8443)
	waitErr := waitPort(port, 5*time.Second)
	_, pidErr := proc.PID(a.h.FrankenPHPpid(), a.franken)
	if waitErr == nil && pidErr == nil {
		// The daemon can still be in its init window, one bind error
		// away from dying; it must still be alive after a settle.
		time.Sleep(500 * time.Millisecond)
		_, pidErr = proc.PID(a.h.FrankenPHPpid(), a.franken)
		if pidErr == nil {
			return nil
		}
	}
	log := filepath.Join(a.h.Log(), "frankenphp.log")
	if pidErr != nil && portAnswers(port) {
		return fmt.Errorf("port %d is held by another process, so frankenphp failed to bind; free it or set DEVENV_HTTPS_PORT. Ports under 1024 need root or setcap cap_net_bind_service=+ep on the frankenphp binary; see %s", port, log)
	}
	if pidErr != nil {
		return fmt.Errorf("daemon failed to start; see %s", log)
	}
	return fmt.Errorf("daemon is running but not answering on port %d; see %s", port, log)
}

func portAnswers(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}

func (a *app) up(dir string) error {
	if _, err := site.Discover(dir); err != nil {
		return err
	}
	if err := a.db.EnsureInstalled(); err != nil {
		return err
	}
	if err := a.db.Start(); err != nil {
		return err
	}
	if err := a.db.EnsureDatabase(projectDB(dir)); err != nil {
		return err
	}
	if err := a.reg.Add(dir); err != nil {
		return err
	}
	if err := a.reload(); err != nil {
		// roll back so a collision or bad render leaves no phantom
		// entry, then reload again: the other projects must not stay
		// offline because this one failed.
		_ = a.reg.Remove(dir)
		_ = a.reload()
		return err
	}
	fmt.Printf("serving %s (db %s):\n", dir, a.db.DSN(projectDB(dir)))
	projects, _ := a.discoverAll()
	for _, s := range projects[dir] {
		fmt.Printf("  %s\n", siteURL(s.Host()))
	}
	return nil
}

func (a *app) down(dir string) error {
	if !a.reg.Has(dir) {
		return fmt.Errorf("%s is not served by lserve (see: lserve list)", dir)
	}
	if err := a.reg.Remove(dir); err != nil {
		return err
	}
	if err := a.reload(); err != nil {
		return err
	}
	if len(a.reg.Projects) == 0 {
		fmt.Println("nothing served; daemon and mysql stopped")
	} else {
		fmt.Printf("stopped serving %s\n", dir)
	}
	return nil
}

func (a *app) restart(dir string) error {
	if !a.reg.Has(dir) {
		return fmt.Errorf("%s is not served by lserve (see: lserve list)", dir)
	}
	if err := a.reload(); err != nil {
		return err
	}
	fmt.Println("reloaded")
	return nil
}

func (a *app) list() error {
	daemonPID, daemonErr := proc.PID(a.h.FrankenPHPpid(), a.franken)
	items, err := a.listItems()
	if err != nil {
		return err
	}
	if !interactive() {
		fmt.Printf("frankenphp daemon: %s\n", daemonState(daemonPID, daemonErr == nil))
		for _, it := range items {
			fmt.Printf("%-40s %s\n", it.Dir, strings.Join(it.Sites, ", "))
		}
		return nil
	}
	return tui.RunProjects(items, daemonState(daemonPID, daemonErr == nil), func(dir, action string) string {
		var err error
		switch action {
		case tui.ActionRestart:
			err = a.restart(dir)
		case tui.ActionStop:
			err = a.down(dir)
		}
		if err != nil {
			return err.Error()
		}
		return "done"
	})
}

func (a *app) listItems() ([]tui.ProjectItem, error) {
	var items []tui.ProjectItem
	for _, dir := range a.reg.Sorted() {
		sites, err := site.Discover(dir)
		if err != nil {
			items = append(items, tui.ProjectItem{Dir: dir, Sites: []string{"(unreadable: " + err.Error() + ")"}})
			continue
		}
		var hosts []string
		for _, s := range sites {
			hosts = append(hosts, siteURL(s.Host()))
		}
		items = append(items, tui.ProjectItem{Dir: dir, Sites: hosts})
	}
	return items, nil
}

func (a *app) logs(args []string) error {
	name := "frankenphp.log"
	if len(args) > 0 && args[0] == "mailpit" {
		name = "mailpit.log"
	}
	// Tail the last 64KB instead of loading the whole file: these logs
	// grow unbounded and reach hundreds of MB.
	f, err := os.Open(filepath.Join(a.h.Log(), name))
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	const tail = 65536
	offset := stat.Size() - tail
	if offset < 0 {
		offset = 0
	}
	buf := make([]byte, stat.Size()-offset)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

func (a *app) runDB(database string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: lserve db <client|import <dump.sql>|dump [file]|dsn>")
	}
	if args[0] == "dsn" {
		fmt.Println(a.db.DSN(database))
		return nil
	}
	if err := a.db.EnsureInstalled(); err != nil {
		return err
	}
	if err := a.db.Start(); err != nil {
		return err
	}
	if err := a.db.EnsureDatabase(database); err != nil {
		return err
	}
	switch args[0] {
	case "client":
		extra := []string{}
		if len(args) > 1 {
			extra = args[1:]
		}
		return a.db.Exec(database, extra)
	case "import":
		if len(args) != 2 {
			return fmt.Errorf("usage: lserve db import <dump.sql>")
		}
		return a.db.Import(database, args[1])
	case "dump":
		file := "-"
		if len(args) == 2 {
			file = args[1]
		} else if len(args) > 2 {
			return fmt.Errorf("usage: lserve db dump [file]")
		}
		return a.db.Dump(database, file)
	}
	return fmt.Errorf("unknown db subcommand %q", args[0])
}

func (a *app) newMailpit() (*mailpit.Mailpit, error) {
	binPath, err := mailpit.EnsureInstalled(a.h.Bin())
	if err != nil {
		return nil, err
	}
	return mailpit.New(binPath, a.h.Run(), a.h.Log(),
		portEnvInt("DEVENV_MAILPIT_UI_PORT", 8025),
		portEnvInt("DEVENV_MAILPIT_SMTP_PORT", 1025)), nil
}

func (a *app) mailpitCmd(args []string) error {
	m, err := a.newMailpit()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "start":
			if err := m.Start(); err != nil {
				return err
			}
			fmt.Printf("mailpit up: %s (smtp 127.0.0.1:%d)\n", m.UIURL(), m.SMTPPort)
			return nil
		case "stop":
			if err := m.Stop(); err != nil {
				return err
			}
			fmt.Println("mailpit stopped")
			return nil
		case "restart":
			_ = m.Stop()
			if err := m.Start(); err != nil {
				return err
			}
			fmt.Printf("mailpit restarted: %s\n", m.UIURL())
			return nil
		case "status":
			return a.mailpitStatus(m)
		}
		return fmt.Errorf("unknown mailpit subcommand %q", args[0])
	}
	if !interactive() {
		return a.mailpitStatus(m)
	}
	return tui.RunMailpit(func() string {
		pid, ok := m.Running()
		if !ok {
			return "stopped"
		}
		return fmt.Sprintf("running (pid %d) at %s, smtp 127.0.0.1:%d", pid, m.UIURL(), m.SMTPPort)
	}, func(action string) string {
		var err error
		switch action {
		case tui.ActionStart:
			err = m.Start()
		case tui.ActionStop:
			err = m.Stop()
		case tui.ActionRestart:
			_ = m.Stop()
			err = m.Start()
		case tui.ActionStatus:
			return a.mailpitStatusString(m)
		}
		if err != nil {
			return err.Error()
		}
		return "done"
	})
}

func (a *app) mailpitStatus(m *mailpit.Mailpit) error {
	fmt.Println(a.mailpitStatusString(m))
	return nil
}

func (a *app) mailpitStatusString(m *mailpit.Mailpit) string {
	pid, ok := m.Running()
	if !ok {
		return "mailpit: stopped (start: lserve mailpit start)"
	}
	return fmt.Sprintf("mailpit: running (pid %d), UI %s, SMTP 127.0.0.1:%d", pid, m.UIURL(), m.SMTPPort)
}

func daemonState(pid int, ok bool) string {
	if !ok {
		return "stopped"
	}
	return fmt.Sprintf("running (pid %d)", pid)
}

func interactive() bool {
	if os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func waitPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("daemon not accepting connections on %d", port)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// siteURL prints a clickable site address; the port is omitted only
// for the real 443 default.
func siteURL(host string) string {
	if port := portEnvInt("DEVENV_HTTPS_PORT", 8443); port != 443 {
		return fmt.Sprintf("https://%s:%d", host, port)
	}
	return "https://" + host
}

func portEnvInt(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// projectDB is the database name for the project: its directory name,
// sanitized to MySQL identifier charset.
func projectDB(projectDir string) string {
	base := filepath.Base(projectDir)
	var b strings.Builder
	for _, r := range base {
		// Hyphens are legal in backticked MySQL identifiers; dropping
		// them would collide "my-site" and "mysite" on one database.
		if r == '_' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	if name := b.String(); name != "" {
		return name
	}
	return "lserve"
}
