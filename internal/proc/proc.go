// Package proc owns detached daemon lifecycles (frankenphp, mailpit):
// start with a pidfile, stop by signal, status by pid liveness. A
// pidfile can outlive a reboot or a kill -9, so liveness re-validates
// the process identity, not just the pid.
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

// Start launches bin with args, detached from the calling terminal,
// logging to logFile, recording the pid in pidPath. Refuses when the
// pidfile already names a live process. Returns once the process is
// confirmed alive.
func Start(bin string, args []string, pidPath, logFile string) error {
	if _, err := pidLive(pidPath, bin); err == nil {
		return fmt.Errorf("already running (pidfile %s)", pidPath)
	}
	_ = os.Remove(pidPath)

	logf, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()

	cmd := exec.Command(bin, args...)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		cmd.Process.Kill()
		return err
	}
	// The child is never reaped (no Wait), so a crashed child stays a
	// zombie for the life of this process: pidLive must reject zombies
	// or this loop would see a dead start as alive.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := pidLive(pidPath, bin); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s exited right after start; see %s", bin, logFile)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Stop signals the process named by pidPath and waits for it to die,
// escalating to SIGKILL.
func Stop(pidPath, bin string) error {
	pid, err := pidLive(pidPath, bin)
	if err != nil {
		_ = os.Remove(pidPath)
		return ErrNotRunning
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	for range 50 {
		if !alive(pid) {
			_ = os.Remove(pidPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return err
	}
	for range 20 {
		if !alive(pid) {
			_ = os.Remove(pidPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("pid %d ignored SIGKILL", pid)
}

// PID returns the live pid from pidPath, or ErrNotRunning.
func PID(pidPath, bin string) (int, error) { return pidLive(pidPath, bin) }

// alive reports whether pid exists in any state, zombie included; only
// used right after we signaled the process ourselves.
func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// pidLive reads the pidfile and confirms the pid is the expected
// process. A stale pidfile whose pid is dead, or belongs to an
// unrelated process, is removed and reported as not running.
func pidLive(pidPath, bin string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, ErrNotRunning
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(pidPath)
		return 0, ErrNotRunning
	}
	// /proc is Linux-only; ps works on both Linux and macOS. Identity
	// check matters because a stale pidfile can point at an unrelated
	// process that reused the pid. The stat field rejects zombies,
	// which this package never reaps after cmd.Start.
	out, err := exec.Command("ps", "-o", "stat=", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	fields := strings.Fields(string(out))
	if err != nil || len(fields) < 2 || strings.HasPrefix(fields[0], "Z") || !strings.Contains(fields[1], filepath.Base(bin)) {
		_ = os.Remove(pidPath)
		return 0, ErrNotRunning
	}
	return pid, nil
}
