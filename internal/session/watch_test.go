package session

import (
	"errors"
	"testing"
	"time"
)

func TestWatchStopsWhenLoginSessionDisappears(t *testing.T) {
	calls := 0
	stopped := false
	err := Watch("session-1", func() (string, error) {
		calls++
		if calls == 1 {
			return "session-1", nil
		}
		return "", errors.New("session gone")
	}, func() error {
		stopped = true
		return nil
	}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !stopped || calls != 2 {
		t.Fatalf("stopped=%v calls=%d", stopped, calls)
	}
}

func TestWatchStopsWhenLoginSessionChanges(t *testing.T) {
	stopped := false
	err := Watch("old", func() (string, error) { return "new", nil }, func() error {
		stopped = true
		return nil
	}, time.Millisecond)
	if err != nil || !stopped {
		t.Fatalf("stopped=%v err=%v", stopped, err)
	}
}
