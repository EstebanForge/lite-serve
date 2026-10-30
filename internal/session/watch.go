// Package session ties background daemons to the login session that
// started them, not to the lifetime of the calling terminal.
package session

import (
	"fmt"
	"time"
)

// Watch requires repeated lookup failures before cleanup so a logind
// restart does not stop healthy sites. Identity changes stop immediately.
func Watch(id string, current func() (string, error), cleanup func() error, interval time.Duration) error {
	if id == "" {
		return fmt.Errorf("empty login session id")
	}
	failures := 0
	for {
		time.Sleep(interval)
		got, err := current()
		stop, next := watchDecision(id, failures, got, err)
		if stop {
			return cleanup()
		}
		failures = next
	}
}

func watchDecision(id string, failures int, got string, err error) (bool, int) {
	if err == nil {
		return got != id, 0
	}
	failures++
	return failures >= 3, failures
}
