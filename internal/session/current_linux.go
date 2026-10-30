package session

import (
	"fmt"
	"os/exec"
	"strings"
)

// Current resolves the caller's logind session. "auto" also works when
// desktop environment variables were stripped from the shell.
func Current() (string, error) {
	out, err := exec.Command("loginctl", "show-session", "auto", "--property=Id", "--property=Active", "--property=State").Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine logind session: %w", err)
	}
	id, err := parseLinuxSession(string(out))
	if err != nil {
		return "", err
	}
	return id, nil
}

func parseLinuxSession(out string) (string, error) {
	properties := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			properties[key] = value
		}
	}
	id, state := properties["Id"], properties["State"]
	if id == "" {
		return "", fmt.Errorf("cannot determine logind session")
	}
	if state != "active" && state != "online" {
		return "", fmt.Errorf("logind session %s is %s", id, state)
	}
	return id, nil
}
