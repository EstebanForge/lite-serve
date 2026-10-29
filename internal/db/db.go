// Package db runs a per-user MySQL 8.4 from a pinned tarball (Linux)
// or a system mysqld from PATH (macOS, where upstream ships no tarball;
// MariaDB works too, with flavor-specific init flags). The datadir
// lives in the state dir and is shared across projects; each project
// gets one database named after its directory.
// Dev-only by design: loopback bind, root with empty password.
package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/EstebanForge/lite-serve/internal/bin"
)

const mysqlVersion = "8.4.5"

// Embedded server per platform; the host mysqld is never used.
var (
	linuxMySQL = asset{
		url: "https://dev.mysql.com/get/Downloads/MySQL-8.4/mysql-8.4.5-linux-glibc2.28-x86_64-minimal.tar.xz",
		sha: "517ca6b9332cc8820c871834fa63e94ee9351ee39993960c04230ce9d194d8d3",
	}
	// macOS: Homebrew bottles (mariadb links brew openssl@4 and pcre2,
	// so those ship too; DYLD_LIBRARY_PATH wires them at runtime). The
	// blob sha256 pins the bytes, the formula/version the contents.
	darwinBottles = []bottle{
		{"mariadb", "mariadb", "https://ghcr.io/v2/homebrew/core/mariadb/blobs/sha256:600e667288555d494f99a1cf3f916bfbaf953b8dfe48a6650011bd5c574a9e94",
			"600e667288555d494f99a1cf3f916bfbaf953b8dfe48a6650011bd5c574a9e94"},
		{"openssl/4", "openssl4", "https://ghcr.io/v2/homebrew/core/openssl/4/blobs/sha256:6fcbad388ef13f6f9481e91676e2ec1b49f0a3dbd473e838297a3c8850283978",
			"6fcbad388ef13f6f9481e91676e2ec1b49f0a3dbd473e838297a3c8850283978"},
		{"pcre2", "pcre2", "https://ghcr.io/v2/homebrew/core/pcre2/blobs/sha256:0ec47d18c50f5770db2b3f07b5b85644284cc6ee8e07e23b11d2727e95932b77",
			"0ec47d18c50f5770db2b3f07b5b85644284cc6ee8e07e23b11d2727e95932b77"},
	}
)

type asset struct{ url, sha string }

// bottle is a pinned Homebrew bottle: formula is the ghcr token scope
// ("openssl/4" for openssl@4), name the local deps dir.
type bottle struct {
	formula string
	name    string
	url     string
	sha     string
}

type DB struct {
	Base   string // dir holding bin/mysqld
	Run    string // pidfile, socket
	Data   string // datadir
	LogDir string
	Port   int
	flavor string // "mysql" or "mariadb", detected from mysqld --version
}

func New(homeRoot, runDir, logDir string, port int) DB {
	return DB{
		Base:   filepath.Join(homeRoot, "mysql"),
		Run:    filepath.Join(runDir, "mysql"),
		Data:   filepath.Join(homeRoot, "mysql-data"),
		LogDir: logDir,
		Port:   port,
	}
}

// Base == "" means a system install (macOS): bare names so
// exec.Command resolves them from PATH; a joined path like "bin/mysqld"
// would not be searched.
func (d DB) mysqld() string {
	if d.Base == "" {
		return "mysqld"
	}
	return filepath.Join(d.Base, "bin", "mysqld")
}
func (d DB) mysql() string {
	if d.Base == "" {
		return "mysql"
	}
	return filepath.Join(d.Base, "bin", "mysql")
}
func (d DB) dump() string {
	if d.Base == "" {
		return "mysqldump"
	}
	return filepath.Join(d.Base, "bin", "mysqldump")
}
func (d DB) socket() string { return filepath.Join(d.Run, "mysql.sock") }
func (d DB) pidfile() string {
	return filepath.Join(d.Run, "mysqld.pid")
}

// EnsureInstalled makes the embedded mysqld available, downloading it
// on first run: pinned MySQL tarball on linux-amd64, pinned MariaDB
// brew bottles on darwin-arm64. Anything else fails visibly.
func (d *DB) EnsureInstalled() error {
	if _, err := os.Stat(d.mysqld()); err == nil {
		return nil
	}
	if err := os.MkdirAll(d.Base, 0o755); err != nil {
		return err
	}
	switch runtime.GOOS + "-" + runtime.GOARCH {
	case "linux-amd64":
		return d.installLinux()
	case "darwin-arm64":
		return d.installDarwin()
	}
	return fmt.Errorf("no embedded database server for %s-%s (supported: linux-amd64, darwin-arm64)", runtime.GOOS, runtime.GOARCH)
}

func (d *DB) installLinux() error {
	archive := filepath.Join(d.Base, "dist.tar.xz")
	if err := bin.DownloadVerified(archive, linuxMySQL.url, linuxMySQL.sha); err != nil {
		return err
	}
	// stdlib has no xz; system tar handles it.
	if out, err := exec.Command("tar", "xJf", archive, "-C", d.Base, "--strip-components=1").CombinedOutput(); err != nil {
		return fmt.Errorf("extract mysql: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Remove(archive)
}

func (d *DB) installDarwin() error {
	// Bottles unpack to <formula>/<version>/*; strip two components so
	// mariadb lands in Base and its library deps in Base/deps/<name>.
	for _, b := range darwinBottles {
		archive := filepath.Join(d.Base, "bottle-"+b.name+".tar.gz")
		if err := bin.DownloadBottle(archive, b.url, b.sha, b.formula); err != nil {
			return err
		}
		target := d.Base
		if b.name != "mariadb" {
			target = filepath.Join(d.Base, "deps", b.name)
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		if out, err := exec.Command("tar", "xzf", archive, "-C", target, "--strip-components=2").CombinedOutput(); err != nil {
			return fmt.Errorf("extract %s bottle: %v: %s", b.formula, err, strings.TrimSpace(string(out)))
		}
		_ = os.Remove(archive)
	}
	return nil
}

// env returns the environment for running server and client binaries.
// Brew bottles reference their dependency dylibs by Homebrew paths that
// do not exist here; DYLD_LIBRARY_PATH resolves them from the deps we
// shipped, by basename, with no binary patching.
func (d DB) dyld() string {
	return strings.Join([]string{
		filepath.Join(d.Base, "deps", "openssl4", "lib"),
		filepath.Join(d.Base, "deps", "pcre2", "lib"),
	}, ":")
}

func (d DB) env() []string {
	if d.Base == "" || runtime.GOOS != "darwin" {
		return nil
	}
	return append(os.Environ(), "DYLD_LIBRARY_PATH="+d.dyld())
}

// detectFlavor distinguishes MySQL from MariaDB: they share the client
// protocol but not the datadir bootstrap or some server flags.
func (d *DB) detectFlavor() error {
	if d.flavor != "" {
		return nil
	}
	cmd := exec.Command(d.mysqld(), "--version")
	cmd.Env = d.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysqld --version: %w: %s", err, strings.TrimSpace(string(out)))
	}
	d.flavor = "mysql"
	if strings.Contains(strings.ToLower(string(out)), "mariadb") {
		d.flavor = "mariadb"
	}
	return nil
}

// initData creates a fresh datadir with empty root password. MySQL
// bootstraps in-process; MariaDB needs its install-db helper, and
// root must use password auth (not unix_socket) for TCP clients.
func (d *DB) initData() error {
	if err := os.MkdirAll(d.Data, 0o755); err != nil {
		return err
	}
	if d.flavor == "mariadb" {
		tool := "mariadb-install-db"
		if d.Base != "" {
			tool = filepath.Join(d.Base, "bin", "mariadb-install-db")
		} else if p, err := exec.LookPath("mysql_install_db"); err == nil {
			tool = p
		}
		args := []string{"--datadir=" + d.Data,
			"--auth-root-authentication-method=normal"}
		if d.Base != "" {
			// The script searches for helpers from its compiled-in
			// Homebrew basedir, which does not exist here.
			args = append(args, "--basedir="+d.Base)
		}
		var cmd *exec.Cmd
		if len(d.env()) > 0 {
			// mariadb-install-db is a shell script whose shebang runs
			// under SIP-restricted /bin/sh, and dyld strips DYLD_LIBRARY_PATH
			// from restricted processes. Re-export it inside the shell so
			// the bootstrap mariadbd child inherits it.
			cmd = exec.Command("/bin/sh", "-c",
				`export DYLD_LIBRARY_PATH="$1"; shift; exec "$@"`,
				"sh", d.dyld(), tool)
			cmd.Args = append(cmd.Args, args...)
		} else {
			cmd = exec.Command(tool, args...)
		}
		cmd.Env = d.env()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("mariadb-install-db: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	initCmd := exec.Command(d.mysqld(), "--initialize-insecure",
		"--datadir="+d.Data, "--log-error-verbosity=1")
	initCmd.Env = d.env()
	out, err := initCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysqld --initialize-insecure: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Start ensures the datadir is initialized and mysqld is live.
func (d *DB) Start() error {
	if err := os.MkdirAll(d.Run, 0o755); err != nil {
		return err
	}
	if err := d.detectFlavor(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(d.Data, "mysql")); err != nil {
		if err := d.initData(); err != nil {
			return err
		}
	}
	if d.live() {
		return nil
	}
	logf, err := os.OpenFile(filepath.Join(d.LogDir, "mysqld.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	args := []string{
		"--datadir=" + d.Data,
		"--bind-address=127.0.0.1",
		"--port=" + strconv.Itoa(d.Port),
		"--socket=" + d.socket(),
		"--pid-file=" + d.pidfile(),
		"--log_error=" + filepath.Join(d.LogDir, "mysqld-error.log"),
	}
	if d.flavor != "mariadb" {
		// MariaDB aborts on this MySQL-only X-plugin flag
		args = append(args, "--mysqlx=OFF")
	} else if d.Base != "" {
		// The bottle has the Cellar path compiled in, which does not
		// exist here; point at the messages we actually shipped.
		args = append(args, "--lc-messages-dir="+filepath.Join(d.Base, "share", "mysql"))
	}
	if pluginDir := filepath.Join(d.Base, "lib", "plugin"); d.Base != "" && dirExists(pluginDir) {
		args = append(args, "--plugin-dir="+pluginDir)
	}
	cmd := exec.Command(d.mysqld(), args...)
	cmd.Env = d.env()
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Drop any stale pidfile first: the wait loop reads it, and a dead
	// old pid there would abort startup while the new daemon forks.
	_ = os.Remove(d.pidfile())
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mysqld: %w", err)
	}
	for range 100 {
		if d.ping() {
			return nil
		}
		// A pidfile that exists but whose process is gone means a real
		// startup failure. No pidfile yet just means mysqld has not
		// finished forking; the pid from cmd.Start may not match the
		// final daemon pid.
		if data, err := os.ReadFile(d.pidfile()); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && !pidAlive(pid) {
				return fmt.Errorf("mysqld exited during startup; see %s", filepath.Join(d.LogDir, "mysqld-error.log"))
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("mysqld did not answer ping in 20s; see %s", filepath.Join(d.LogDir, "mysqld-error.log"))
}

// Running reports whether mysqld is live.
func (d *DB) Running() bool { return d.live() }

// Stop signals mysqld if live.
func (d *DB) Stop() error {
	pid := d.pid()
	if pid == 0 {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	for range 50 {
		if !d.live() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return err
	}
	for range 20 {
		if !d.live() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("mysqld pid %d ignored SIGKILL", pid)
}

// EnsureDatabase creates the project database if absent. Root over
// loopback with empty password (initialize-insecure), dev-only.
func (d *DB) EnsureDatabase(name string) error {
	out, err := d.exec("CREATE DATABASE IF NOT EXISTS `" + name + "`")
	if err != nil {
		// ExitError alone is bare "exit status 1"; the MySQL message
		// is in the output.
		return fmt.Errorf("create database %s: %v: %s", name, err, strings.TrimSpace(out))
	}
	return nil
}

// Exec runs the mysql client with args and connects to the project DB.
func (d *DB) Exec(database string, args []string) error {
	full := append([]string{
		"-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", database,
	}, args...)
	cmd := exec.Command(d.mysql(), full...)
	cmd.Env = d.env()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Import loads a SQL dump into the project database.
func (d *DB) Import(database, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	full := []string{
		"-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", database,
	}
	cmd := exec.Command(d.mysql(), full...)
	cmd.Env = d.env()
	cmd.Stdin = f
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Dump writes the project database to file ("-" for stdout). Files are
// written via a temp sibling and renamed only on success, so a failed
// dump never truncates an existing backup.
func (d *DB) Dump(database, file string) error {
	args := []string{
		"-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", database,
	}
	if file == "-" {
		cmd := exec.Command(d.dump(), args...)
		cmd.Env = d.env()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	tmp := file + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	cmd := exec.Command(d.dump(), args...)
	cmd.Env = d.env()
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, file)
}

func (d *DB) DSN(database string) string {
	return fmt.Sprintf("mysql://root@127.0.0.1:%d/%s", d.Port, database)
}

// pid returns the pidfile pid, 0 when absent or stale. Like the web
// server lifecycle, identity is verified: a bare kill(pid, 0) check
// could SIGTERM an unrelated process that reused the pid.
func (d *DB) pid() int {
	data, err := os.ReadFile(d.pidfile())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil || !strings.Contains(string(out), "mysqld") {
		return 0
	}
	return pid
}

func (d *DB) live() bool { return d.pid() != 0 }

// ping asks mysqld for its status over the admin socket path; using the
// mysqladmin binary from the same install keeps versions matched.
func (d *DB) ping() bool {
	admin := filepath.Join(d.Base, "bin", "mysqladmin")
	if d.Base == "" {
		admin = "mysqladmin"
	}
	cmd := exec.Command(admin, "-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", "ping")
	cmd.Env = d.env()
	return cmd.Run() == nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func (d *DB) exec(sql string) (string, error) {
	args := []string{
		"-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", "-e", sql,
	}
	cmd := exec.Command(d.mysql(), args...)
	cmd.Env = d.env()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
