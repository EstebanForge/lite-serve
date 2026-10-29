// Package db runs a per-user MySQL 8.4 from a pinned tarball (Linux)
// or a system mysqld from PATH (macOS, where upstream ships no tarball).
// The datadir lives in the state dir and is shared across projects;
// each project gets one database named after its directory.
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

const (
	mysqlVersion = "8.4.5"
	assetSHA256  = "517ca6b9332cc8820c871834fa63e94ee9351ee39993960c04230ce9d194d8d3"
	assetURL     = "https://dev.mysql.com/get/Downloads/MySQL-8.4/" +
		"mysql-8.4.5-linux-glibc2.28-x86_64-minimal.tar.xz"
)

type DB struct {
	Base   string // dir holding bin/mysqld
	Run    string // pidfile, socket
	Data   string // datadir
	LogDir string
	Port   int
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

// EnsureInstalled makes the mysqld binary available: pinned tarball on
// Linux amd64, system PATH lookup elsewhere (macOS: brew install mysql).
func (d *DB) EnsureInstalled() error {
	if _, err := os.Stat(d.mysqld()); err == nil {
		return nil
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		if _, err := exec.LookPath("mysqld"); err == nil {
			d.Base = "" // system install: binaries resolved from PATH
			return nil
		}
		return fmt.Errorf("no mysqld found. Linux amd64 gets an automatic pinned MySQL %s; on %s install one: brew install mysql", mysqlVersion, runtime.GOOS)
	}

	if err := os.MkdirAll(d.Base, 0o755); err != nil {
		return err
	}
	archive := filepath.Join(d.Base, "dist.tar.xz")
	if err := bin.DownloadVerified(archive, assetURL, assetSHA256); err != nil {
		return err
	}
	// stdlib has no xz; system tar handles it on both Linux and macOS.
	if out, err := exec.Command("tar", "xJf", archive, "-C", d.Base, "--strip-components=1").CombinedOutput(); err != nil {
		return fmt.Errorf("extract mysql: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Remove(archive)
}

// Start ensures the datadir is initialized and mysqld is live.
func (d *DB) Start() error {
	if err := os.MkdirAll(d.Run, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(d.Data, "mysql")); err != nil {
		if err := os.MkdirAll(d.Data, 0o755); err != nil {
			return err
		}
		out, err := exec.Command(d.mysqld(), "--initialize-insecure",
			"--datadir="+d.Data, "--log-error-verbosity=1").CombinedOutput()
		if err != nil {
			return fmt.Errorf("mysqld --initialize-insecure: %v: %s", err, strings.TrimSpace(string(out)))
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
	cmd := exec.Command(d.mysqld(),
		"--datadir="+d.Data,
		"--bind-address=127.0.0.1",
		"--port="+strconv.Itoa(d.Port),
		"--socket="+d.socket(),
		"--pid-file="+d.pidfile(),
		"--mysqlx=OFF",
		"--log_error="+filepath.Join(d.LogDir, "mysqld-error.log"),
	)
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
	err := exec.Command(admin, "-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", "ping").Run()
	return err == nil
}

func (d *DB) exec(sql string) (string, error) {
	args := []string{
		"-h", "127.0.0.1", "-P", strconv.Itoa(d.Port),
		"-u", "root", "--protocol=TCP", "-e", sql,
	}
	out, err := exec.Command(d.mysql(), args...).CombinedOutput()
	return string(out), err
}

func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
