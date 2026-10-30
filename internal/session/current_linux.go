package session

import (
	"fmt"
	"os/exec"
	"strings"
)

// Current resolves the caller's logind session. "auto" also works when
// desktop environment variables were stripped from the shell.
func Current() (string, error) {
	out, err := exec.Command("loginctl", "show-session", "auto", "--property=Id", "--value").Output()
	id := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("cannot determine logind session: %w", err)
	}
	if id == "" {
		return "", fmt.Errorf("cannot determine logind session")
	}
	return id, nil
}
