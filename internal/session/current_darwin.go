package session

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Current uses the per-login launchd manager pid. It survives terminal
// closure but changes when the GUI login session is replaced.
func Current() (string, error) {
	out, err := exec.Command("launchctl", "managerpid").Output()
	id := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("cannot determine launchd login session: %w", err)
	}
	if _, err := strconv.Atoi(id); err != nil || id == "0" {
		return "", fmt.Errorf("cannot determine launchd login session")
	}
	return id, nil
}
