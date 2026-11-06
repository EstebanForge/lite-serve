package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/EstebanForge/lite-serve/internal/home"
	"github.com/EstebanForge/lite-serve/internal/mailpit"
	"github.com/EstebanForge/lite-serve/internal/ports"
	"github.com/EstebanForge/lite-serve/internal/proc"
)

type portRow struct {
	port  int
	label string
	env   string
}

func portRows() []portRow {
	return []portRow{
		{portEnvInt("DEVENV_HTTP_PORT", 8080), "http", "DEVENV_HTTP_PORT"},
		{portEnvInt("DEVENV_HTTPS_PORT", 8443), "https", "DEVENV_HTTPS_PORT"},
		{portEnvInt("DEVENV_DB_PORT", 3306), "db", "DEVENV_DB_PORT"},
		{portEnvInt("DEVENV_MAILPIT_UI_PORT", 8025), "mailpit-ui", "DEVENV_MAILPIT_UI_PORT"},
		{portEnvInt("DEVENV_MAILPIT_SMTP_PORT", 1025), "mailpit-smtp", "DEVENV_MAILPIT_SMTP_PORT"},
	}
}

// lserve ports prints who holds each lserve port: free, ours (a lserve
// pidfile-owned process), or held by a foreign holder with attribution.
func (a *app) portsReport() error {
	for _, row := range portRows() {
		state, detail := "free", ""
		if ports.Held(row.port) {
			h := ports.Identify(row.port)
			if a.oursAmong(row, h) {
				state, detail = "ours", "lserve"
			} else {
				state, detail = "held", formatHolder(h)
			}
		}
		fmt.Printf("%-6d %-13s %-5s %s\n", row.port, row.label, state, detail)
	}
	return nil
}

// componentPID is the pidfile pid of the lserve process owning a row's
// service, 0 when that service is not running.
func (a *app) componentPID(row portRow) int {
	switch row.label {
	case "http", "https":
		pid, err := proc.PID(a.h.FrankenPHPpid(), a.franken)
		if err != nil {
			return 0
		}
		return pid
	case "db":
		return a.db.PID()
	default:
		pid, ok := mailpitPID(a)
		if !ok {
			return 0
		}
		return pid
	}
}

func mailpitPID(a *app) (int, bool) {
	m := mailpit.New(filepath.Join(a.h.Bin(), "mailpit"), a.h.Run(), a.h.Log(),
		portEnvInt("DEVENV_MAILPIT_UI_PORT", 8025),
		portEnvInt("DEVENV_MAILPIT_SMTP_PORT", 1025))
	return m.Running()
}

// oursAmong decides a clash is ours only when our pidfile pid is among
// the port's actual listeners: a running mysqld on another port (env
// override drift) must not claim this row.
func (a *app) oursAmong(row portRow, h ports.Holder) bool {
	pid := a.componentPID(row)
	if pid == 0 {
		return false
	}
	for _, p := range h.PIDs {
		if p == pid {
			return true
		}
	}
	return false
}

func containerLabel(c ports.Container) string {
	return fmt.Sprintf("%s: %s (%s)", c.Engine, c.Name, c.Image)
}

func plainPidParts(h ports.Holder) []string {
	var parts []string
	for i, pid := range h.PIDs {
		name := h.Process
		if names := strings.Split(h.Process, ", "); i < len(names) {
			name = names[i]
		}
		parts = append(parts, fmt.Sprintf("pid %d %s", pid, name))
	}
	return parts
}

func formatHolder(h ports.Holder) string {
	var parts []string
	for i, c := range h.Containers {
		label := containerLabel(c)
		if i == 0 && len(h.PIDs) > 0 {
			label += fmt.Sprintf(", pid %d %s", h.PIDs[0], h.Process)
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		parts = plainPidParts(h)
	}
	if len(parts) == 0 {
		return "unidentified process (no permission to inspect it)"
	}
	return strings.Join(parts, ", ")
}

// preflight refuses to start services while foreign holders block the
// ports they need. Interactive sessions may clear the port by stopping
// containers or SIGTERM-ing plain processes after an explicit yes;
// lserve never signals its own pidfile-owned processes and never kills
// anything in a non-interactive run.
func (a *app) preflight(rows []portRow) error {
	var in *bufio.Reader
	for _, row := range rows {
		if err := a.resolveRow(row, &in); err != nil {
			return err
		}
	}
	return nil
}

// resolveRow clears one clashing port. A port can need several rounds:
// SO_REUSEPORT puts several listeners on one port, and a container
// with a restart policy respawns. Three rounds cover that; then it
// aborts. The re-identify round after every action shows who actually
// still holds the port, so no stale pid is ever signaled from an old
// attribution.
func (a *app) resolveRow(row portRow, in **bufio.Reader) error {
	for range 3 {
		if !ports.Held(row.port) {
			return nil
		}
		h := ports.Identify(row.port)
		if a.oursAmong(row, h) {
			return nil
		}
		fmt.Printf("port %d (%s) is held by:\n  %s\n", row.port, row.label, formatHolder(h))
		if !interactive() {
			return refuse(row)
		}
		switch {
		case len(h.Containers) > 0:
			labels := make([]string, len(h.Containers))
			for i, c := range h.Containers {
				labels[i] = containerLabel(c)
			}
			fmt.Printf("Stop %s? [y/N] ", strings.Join(labels, ", "))
		case len(h.PIDs) > 0:
			fmt.Printf("Send SIGTERM to %s? [y/N] ", strings.Join(plainPidParts(h), ", "))
		default:
			return refuse(row)
		}
		if *in == nil {
			*in = bufio.NewReader(os.Stdin)
		}
		answer, err := readPromptLine(*in)
		if err != nil || !wantsStop(answer) {
			return refuse(row)
		}
		if len(h.Containers) > 0 {
			if err := ports.StopContainers(h.Containers, row.port); err != nil {
				return err
			}
			names := make([]string, len(h.Containers))
			for i, c := range h.Containers {
				names[i] = c.Name
			}
			fmt.Printf("stopped %s\n", strings.Join(names, ", "))
			continue
		}
		signaled := 0
		for _, pid := range h.PIDs {
			// A 0 pid here would signal lserve's own process group.
			if pid <= 0 {
				continue
			}
			if syscall.Kill(pid, syscall.SIGTERM) == nil {
				signaled++
			}
		}
		if signaled == 0 {
			continue
		}
		if !ports.WaitFree(row.port, 15*time.Second) {
			continue
		}
		return nil
	}
	return refuse(row)
}

func refuse(row portRow) error {
	return fmt.Errorf("port %d (%s) stays held; free it or set %s", row.port, row.label, row.env)
}

func wantsStop(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// earlyPreflight checks the always-needed web ports before app
// construction, so no binary download happens behind a doomed run. The
// db row waits for the in-up preflight, which knows proxy mode.
func earlyPreflight() error {
	h, err := home.Dir()
	if err != nil {
		return err
	}
	a := &app{h: h, franken: filepath.Join(h.Bin(), "frankenphp")}
	return a.preflight(portRows()[:2])
}
