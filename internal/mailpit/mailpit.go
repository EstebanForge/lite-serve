// Package mailpit runs a pinned, per-user Mailpit for local email
// capture: SMTP on 1025, web UI on 8025, loopback only.
package mailpit

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/EstebanForge/lite-devenv/internal/bin"
	"github.com/EstebanForge/lite-devenv/internal/proc"
)

type Mailpit struct {
	BinPath  string
	RunDir   string // pidfile lives here
	LogDir   string
	UIPort   int
	SMTPPort int
}

func New(binPath, runDir, logDir string, uiPort, smtpPort int) *Mailpit {
	return &Mailpit{BinPath: binPath, RunDir: runDir, LogDir: logDir, UIPort: uiPort, SMTPPort: smtpPort}
}

// EnsureInstalled makes the pinned mailpit binary available.
func EnsureInstalled(binDir string) (string, error) {
	return bin.Mailpit(binDir)
}

func (m *Mailpit) pidPath() string { return filepath.Join(m.RunDir, "mailpit.pid") }

// Start launches mailpit detached; no-op when already live.
func (m *Mailpit) Start() error {
	return proc.Start(m.BinPath, []string{
		"--listen", "127.0.0.1:" + strconv.Itoa(m.UIPort),
		"--smtp", "127.0.0.1:" + strconv.Itoa(m.SMTPPort),
		// auth is off unless configured; loopback bind keeps it local
	}, m.pidPath(), filepath.Join(m.LogDir, "mailpit.log"))
}

// Stop signals mailpit if live.
func (m *Mailpit) Stop() error {
	err := proc.Stop(m.pidPath(), m.BinPath)
	if err == proc.ErrNotRunning {
		return nil
	}
	return err
}

// Running reports whether mailpit is live, with its pid.
func (m *Mailpit) Running() (int, bool) {
	pid, err := proc.PID(m.pidPath(), m.BinPath)
	if err != nil {
		return 0, false
	}
	return pid, true
}

// UIURL is where the inbox is browsable.
func (m *Mailpit) UIURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", m.UIPort)
}
