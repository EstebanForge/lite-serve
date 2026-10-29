// Package proc owns the frankenphp daemon lifecycle: start with a
// pidfile, stop by signal, status by pid liveness. A pidfile can
// outlive a reboot or a kill -9, so liveness re-validates the process
// identity, not just the pid.
package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrNotRunning = errors.New("not running")

// Start launches frankenphp with the given Caddyfile, detached from the
// calling terminal, logging to logFile. Refuses when already live.
func Start(bin, runDir, logFile, caddyfile string) error {
	if _, err := pidLive(runDir, bin); err == nil {
		return fmt.Errorf("already running (pidfile %s)", pidfile(runDir))
	}
	_ = os.Remove(pidfile(runDir))

	logf, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()

	cmd := exec.Command(bin, "run", "--config", caddyfile, "--adapter", "caddyfile")
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start frankenphp: %w (need ports 80/443? set DEVENV_HTTP_PORT/DEVENV_HTTPS_PORT)", err)
	}
	if err := os.WriteFile(pidfile(runDir), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		cmd.Process.Kill()
		return err
	}
	// frankenphp validates the Caddyfile and binds the ports before it
	// daemonizes its workers; poll so a bad config or a port conflict is
	// reported instead of a silent death. The child is never reaped, so
	// a crashed frankenphp is a zombie: pidLive must reject zombies or
	// this loop would see it as alive.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := pidLive(runDir, bin); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("frankenphp exited right after start; see %s", logFile)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Stop signals the live frankenphp and waits for it to die.
func Stop(runDir, bin string) error {
	pid, err := pidLive(runDir, bin)
	if err != nil {
		_ = os.Remove(pidfile(runDir))
		return ErrNotRunning
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	for range 50 {
		if err := syscall.Kill(pid, 0); err != nil {
			_ = os.Remove(pidfile(runDir))
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Uncooperative: escalate so stop never leaves a daemon behind.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return err
	}
	for range 20 {
		if err := syscall.Kill(pid, 0); err != nil {
			_ = os.Remove(pidfile(runDir))
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("pid %d ignored SIGKILL", pid)
}

// PID returns the live frankenphp pid, or ErrNotRunning.
func PID(runDir, bin string) (int, error) { return pidLive(runDir, bin) }

func pidfile(runDir string) string { return filepath.Join(runDir, "frankenphp.pid") }

// pidLive reads the pidfile and confirms the pid is a frankenphp
// process. A stale pidfile whose pid is dead, or belongs to an
// unrelated process, is removed and reported as not running.
func pidLive(runDir, bin string) (int, error) {
	data, err := os.ReadFile(pidfile(runDir))
	if err != nil {
		return 0, ErrNotRunning
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(pidfile(runDir))
		return 0, ErrNotRunning
	}
	// /proc is Linux-only; ps works on both Linux and macOS. Identity
	// check matters because a stale pidfile can point at an unrelated
	// process that reused the pid. The stat field rejects zombies, which
	// this package never reaps after cmd.Start.
	out, err := exec.Command("ps", "-o", "stat=", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	fields := strings.Fields(string(out))
	if err != nil || len(fields) < 2 || strings.HasPrefix(fields[0], "Z") || !strings.Contains(fields[1], filepath.Base(bin)) {
		_ = os.Remove(pidfile(runDir))
		return 0, ErrNotRunning
	}
	return pid, nil
}
