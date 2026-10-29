package registry

import (
	"sync"
	"testing"
)

func TestAddRemoveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Add("/p1", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("/p2", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("/p1", ""); err != nil { // duplicate is a no-op
		t.Fatal(err)
	}
	fresh, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.Sorted(); len(got) != 2 || got[0] != "/p1" || got[1] != "/p2" {
		t.Fatalf("want [/p1 /p2], got %v", got)
	}
	if err := fresh.Remove("/p1"); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Remove("/nope"); err != ErrNotRegistered {
		t.Errorf("removing unknown dir: want ErrNotRegistered, got %v", err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Sorted(); len(got) != 1 || got[0] != "/p2" {
		t.Fatalf("want [/p2], got %v", got)
	}
}

// Adds from separate Open handles (as two terminals would do) must not
// lose each other's entry; the flock makes the second re-read the file
// the first wrote.
func TestConcurrentAddsLoseNothing(t *testing.T) {
	dir := t.TempDir()
	const n = 8
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := Open(dir)
			if err != nil {
				t.Error(err)
				return
			}
			if err := r.Add("/proj-"+string(rune('a'+i)), ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(r.Sorted()); got != n {
		t.Fatalf("want %d projects after concurrent adds, got %d", n, got)
	}
}
