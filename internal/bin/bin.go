// Package bin downloads and verifies pinned external binaries
// (frankenphp, mariadb) into the state dir. Every entry carries a
// sha256 that must match the published asset byte for byte; a failed
// verify aborts start with the temp file left for inspection.
package bin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const frankenVersion = "v1.13.4"

type asset struct {
	url    string
	sha256 string
}

var frankenAssets = map[string]asset{
	"linux-amd64": {
		"https://github.com/dunglas/frankenphp/releases/download/" + frankenVersion + "/frankenphp-linux-x86_64",
		"eddb171ac2caeb1bc54eb858f9ac01d5936582dc15de1990e84b9abedef851dd",
	},
	"darwin-arm64": {
		"https://github.com/dunglas/frankenphp/releases/download/" + frankenVersion + "/frankenphp-mac-arm64",
		"90efcc5df08f955f1e50aec3186334b53d4847a7fd19241d3f43d0cfcf3382e3",
	},
}

// frankenAsset picks the pinned frankenphp asset for the running OS/arch.
func frankenAsset(goos, goarch string) (asset, error) {
	key := goos + "-" + goarch
	if a, ok := frankenAssets[key]; ok {
		return a, nil
	}
	return asset{}, fmt.Errorf("no pinned frankenphp asset for %s", key)
}

// FrankenPHP returns the path to the verified frankenphp binary,
// downloading it on first use.
func FrankenPHP(binDir string) (string, error) {
	dest := filepath.Join(binDir, "frankenphp")
	if isVerified(dest, frankenAssets[runtime.GOOS+"-"+runtime.GOARCH].sha256) {
		return dest, nil
	}
	a, err := frankenAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := downloadVerified(dest, a.url, a.sha256); err != nil {
		return "", fmt.Errorf("frankenphp: %w", err)
	}
	return dest, nil
}

// IsVerified reports whether dest exists and matches wantSHA.
func IsVerified(dest, wantSHA string) bool { return isVerified(dest, wantSHA) }

// DownloadVerified exposes verified download for other packages (mariadb, mysql).
func DownloadVerified(dest, url, wantSHA string) error {
	return downloadVerified(dest, url, wantSHA)
}

// isVerified reports whether dest exists and matches wantSHA.
func isVerified(dest, wantSHA string) bool {
	f, err := os.Open(dest)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == wantSHA
}

// downloadVerified streams url to dest (via a .tmp sibling) and rejects
// the result unless its sha256 matches. Partial downloads never replace
// an existing verified binary.
func downloadVerified(dest, url, wantSHA string) error {
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	resp, err := http.Get(url)
	if err != nil {
		out.Close()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out.Close()
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), resp.Body); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return fmt.Errorf("sha256 mismatch: got %s, want %s (left at %s)", got, wantSHA, tmp)
	}
	return os.Rename(tmp, dest)
}
