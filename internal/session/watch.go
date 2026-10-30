// Package session ties background daemons to the login session that
// started them, not to the lifetime of the calling terminal.
package session

import (
	"fmt"
	"time"
)

// Watch runs cleanup as soon as the original login session disappears
// or is replaced. Lookup errors stop the daemons: failing open would
// leave them running after logout.
func Watch(id string, current func() (string, error), cleanup func() error, interval time.Duration) error {
	if id == "" {
		return fmt.Errorf("empty login session id")
	}
	for {
		time.Sleep(interval)
		got, err := current()
		if err != nil || got != id {
			return cleanup()
		}
	}
}
