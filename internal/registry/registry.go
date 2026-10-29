// Package registry tracks which project directories lserve is serving.
// One JSON file in the state dir; the global frankenphp daemon renders
// every registered project's sites into one Caddyfile, which is what
// lets all projects share port 443 like a valet install.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

var ErrNotRegistered = errors.New("not registered")

type Project struct {
	Dir     string    `json:"dir"`
	AddedAt time.Time `json:"added_at"`
}

type Registry struct {
	path     string
	Projects []Project `json:"projects"`
}

func Open(stateDir string) (*Registry, error) {
	r := &Registry{path: filepath.Join(stateDir, "registry.json")}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("corrupt registry %s: %w", r.path, err)
	}
	return r, nil
}

func (r *Registry) Has(dir string) bool {
	_, err := r.find(dir)
	return err == nil
}

// Add and Remove are whole read-modify-write cycles. Two terminals
// running lserve at once would otherwise lose each other's entry to a
// last-write-wins race, so the lock is held across re-read + mutate +
// save; the in-memory copy from Open is refreshed under the lock.
func (r *Registry) Add(dir string) error {
	return r.locked(func() error {
		if err := r.reread(); err != nil {
			return err
		}
		if _, err := r.find(dir); err == nil {
			return nil
		}
		r.Projects = append(r.Projects, Project{Dir: dir, AddedAt: time.Now()})
		return r.save()
	})
}

func (r *Registry) Remove(dir string) error {
	return r.locked(func() error {
		if err := r.reread(); err != nil {
			return err
		}
		i, err := r.find(dir)
		if err != nil {
			return err
		}
		r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
		return r.save()
	})
}

func (r *Registry) locked(fn func() error) error {
	lf, err := os.OpenFile(r.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	return fn()
}

// reread reloads the file into memory; only valid under locked().
func (r *Registry) reread() error {
	fresh, err := Open(filepath.Dir(r.path))
	if err != nil {
		return err
	}
	r.Projects = fresh.Projects
	return nil
}

// Sorted returns registered directories, stable order for rendering.
func (r *Registry) Sorted() []string {
	dirs := make([]string, 0, len(r.Projects))
	for _, p := range r.Projects {
		dirs = append(dirs, p.Dir)
	}
	sort.Strings(dirs)
	return dirs
}

func (r *Registry) find(dir string) (int, error) {
	for i, p := range r.Projects {
		if p.Dir == dir {
			return i, nil
		}
	}
	return -1, ErrNotRegistered
}

func (r *Registry) save() error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	// pid in the temp name so concurrent writers never share a file
	tmp := fmt.Sprintf("%s.%d.tmp", r.path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
