package session

import (
	"errors"
	"testing"
)

func TestWatchDecisionToleratesTransientErrors(t *testing.T) {
	failures := 0
	var stop bool
	for i := 0; i < 2; i++ {
		stop, failures = watchDecision("session-1", failures, "", errors.New("bus unavailable"))
		if stop {
			t.Fatal("transient error stopped the watcher")
		}
	}
	stop, _ = watchDecision("session-1", failures, "", errors.New("bus unavailable"))
	if !stop {
		t.Fatal("persistent session lookup failure did not stop the watcher")
	}
}

func TestWatchDecisionIdentityAndRecovery(t *testing.T) {
	stop, failures := watchDecision("session-1", 2, "session-1", nil)
	if stop || failures != 0 {
		t.Fatalf("successful lookup did not reset failures: stop=%v failures=%d", stop, failures)
	}
	stop, _ = watchDecision("session-1", 0, "session-2", nil)
	if !stop {
		t.Fatal("session identity change did not stop the watcher")
	}
}

func TestParseLinuxSession(t *testing.T) {
	id, err := parseLinuxSession("State=active\nId=2\nActive=yes\n")
	if err != nil || id != "2" {
		t.Fatalf("active session rejected: id=%q err=%v", id, err)
	}
	for _, output := range []string{"Id=2\nState=closing\nActive=yes\n", "Id=2\nState=\nActive=yes\n"} {
		if _, err := parseLinuxSession(output); err == nil {
			t.Fatalf("inactive session accepted: %q", output)
		}
	}
	id, err = parseLinuxSession("Id=2\nActive=no\nState=online\n")
	if err != nil || id != "2" {
		t.Fatalf("background online session rejected: id=%q err=%v", id, err)
	}
}
