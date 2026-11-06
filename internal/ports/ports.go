// Package ports answers "who is holding this port": a listener probe
// first, then best-effort attribution to a pid and, when the holder is
// a docker or podman published port, to a named container. It only
// gathers facts and stops containers it is told to; the policy of when
// to kill or stop lives in cmd/lserve.
package ports

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Container is a running container whose published host port collides
// with a port lserve needs.
type Container struct {
	Engine string // docker or podman
	ID     string
	Name   string
	Image  string
}

// Holder describes everything lserve managed to learn about a port.
type Holder struct {
	Port       int
	PIDs       []int
	Process    string      // comma-joined listener names; empty when unreadable
	Containers []Container // every container publishing this port
}

var ssUser = regexp.MustCompile(`\("([^"]+)",pid=(\d+)`)

// Held reports whether anything accepts connections on port, probing
// both stacks so IPv4-only and IPv6-only listeners both count.
func Held(port int) bool {
	var d net.Dialer
	d.Timeout = 150 * time.Millisecond
	for _, addr := range []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("[::1]:%d", port)} {
		c, err := d.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return true
		}
	}
	return false
}

// WaitFree polls until nothing answers on port or the timeout passes.
func WaitFree(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !Held(port) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Identify finds the holder of a port. Every failure to attribute the
// holder degrades to a less specific Holder; it never fails outright,
// because a clash is actionable even when the holder stays anonymous.
func Identify(port int) Holder {
	h := Holder{Port: port}
	li := listeners(port)
	h.PIDs = li.pids
	h.Process = li.names
	h.Containers = mergeContainers(containerMatches("docker", port), containerMatches("podman", port))
	return h
}

// StopContainers stops each container by ID with its own engine, then
// waits for the port to actually free: a restart policy can respawn a
// container right after a successful stop, and that must not pass as
// success.
func StopContainers(cs []Container, port int) error {
	var errs []error
	for _, c := range cs {
		if err := exec.Command(c.Engine, "stop", c.ID).Run(); err != nil {
			errs = append(errs, fmt.Errorf("%s stop %s (%s): %w", c.Engine, c.Name, c.ID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("stop containers: %v", errs)
	}
	if !WaitFree(port, 10*time.Second) {
		return fmt.Errorf("stopped %s but port %d is still held", strings.Join(names(cs), ", "), port)
	}
	return nil
}

func names(cs []Container) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

type listenerInfo struct {
	pids  []int
	names string
}

func listeners(port int) listenerInfo {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
		if err != nil {
			return listenerInfo{}
		}
		return parseLsof(string(out), port)
	default:
		out, err := exec.Command("ss", "-Htlnp").Output()
		if err != nil {
			return listenerInfo{}
		}
		return parseSs(string(out), port)
	}
}

func parseSs(out string, port int) listenerInfo {
	var li listenerInfo
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		if _, p, err := net.SplitHostPort(f[3]); err != nil || p != strconv.Itoa(port) {
			continue
		}
		for _, m := range ssUser.FindAllStringSubmatch(line, -1) {
			if !seen[m[2]] {
				seen[m[2]] = true
				li.pids = append(li.pids, atoi(m[2]))
				li.names = joinName(li.names, m[1])
			}
		}
	}
	return li
}

func parseLsof(out string, port int) listenerInfo {
	var li listenerInfo
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 9 || f[len(f)-1] != "(LISTEN)" {
			continue
		}
		name := f[len(f)-2]
		if _, p, err := net.SplitHostPort(name); err != nil || p != strconv.Itoa(port) {
			continue
		}
		pid := atoi(f[1])
		// lsof columns shift when a process name contains spaces; a
		// non-numeric pid field must drop the line, not signal pid 0.
		if pid <= 0 {
			continue
		}
		if !seen[f[1]] {
			seen[f[1]] = true
			li.pids = append(li.pids, pid)
			li.names = joinName(li.names, f[0])
		}
	}
	return li
}

func containerMatches(engine string, port int) []Container {
	out, err := exec.Command(engine, "ps", "--format",
		"{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Ports}}").Output()
	if err != nil {
		return nil
	}
	return parsePs(string(out), engine, port)
}

func parsePs(out, engine string, port int) []Container {
	var cs []Container
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 || !publishes(f[3], port) {
			continue
		}
		cs = append(cs, Container{Engine: engine, ID: f[0], Name: f[1], Image: f[2]})
	}
	return cs
}

func mergeContainers(groups ...[]Container) []Container {
	var out []Container
	seen := map[string]bool{}
	for _, g := range groups {
		for _, c := range g {
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// publishes reports whether a docker/podman ps Ports field maps a host
// port. Exposed-but-unpublished entries ("9000/tcp") never match.
func publishes(fields string, host int) bool {
	for _, m := range strings.Split(fields, ",") {
		m = strings.TrimSpace(m)
		hostPart, _, ok := strings.Cut(m, "->")
		if !ok {
			continue
		}
		if _, p, err := net.SplitHostPort(strings.TrimSpace(hostPart)); err == nil && p == strconv.Itoa(host) {
			return true
		}
	}
	return false
}

func joinName(acc, name string) string {
	if acc == "" {
		return name
	}
	return acc + ", " + name
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
