// Package bin downloads and verifies pinned external binaries
// (frankenphp, mariadb) into the state dir. Every entry carries a
// sha256 that must match the published asset byte for byte; a failed
// verify aborts start with the temp file left for inspection.
package bin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

const mailpitVersion = "v1.32.0"

// mailpit ships as a small tarball containing the binary plus docs.
var mailpitAssets = map[string]asset{
	"linux-amd64": {
		"https://github.com/axllent/mailpit/releases/download/" + mailpitVersion + "/mailpit-linux-amd64.tar.gz",
		"4b5993c978e8604c70d43927e2ad103b311bba3b1822fa0cf1d4ee4ec890a93c",
	},
	"darwin-arm64": {
		"https://github.com/axllent/mailpit/releases/download/" + mailpitVersion + "/mailpit-darwin-arm64.tar.gz",
		"23709bd727f20da2290de6a2a40c4685c48a3127586d77370efd1a3a595f9ef6",
	},
}

// frankenAsset picks the pinned frankenphp asset for the running OS/arch.
func frankenAsset(goos, goarch string) (asset, error) {
	key := goos + "-" + goarch
	if a, ok := frankenAssets[key]; ok {
		return a, nil
	}
	return asset{}, fmt.Errorf("no pinned frankenphp asset for %s (supported: linux-amd64, darwin-arm64; Windows via WSL)", key)
}

// FrankenPHP returns the path to the verified frankenphp binary,
// downloading it on first use.
func FrankenPHP(binDir string) (string, error) {
	dest := filepath.Join(binDir, "frankenphp")
	a, err := frankenAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if isVerified(dest, a.sha256) {
		return dest, nil
	}
	if err := downloadVerified(dest, a.url, a.sha256); err != nil {
		return "", fmt.Errorf("frankenphp: %w", err)
	}
	return dest, nil
}

// Mailpit returns the path to the verified mailpit binary, downloading
// and extracting the pinned tarball on first use.
func Mailpit(binDir string) (string, error) {
	dest := filepath.Join(binDir, "mailpit")
	if _, err := os.Stat(dest); err == nil {
		return dest, nil // extraction lands via rename, so presence is enough
	}
	a, err := pickAsset(mailpitAssets)
	if err != nil {
		return "", fmt.Errorf("mailpit: %w", err)
	}
	archive := filepath.Join(binDir, "mailpit-dist.tar.gz")
	if err := downloadVerified(archive, a.url, a.sha256); err != nil {
		return "", fmt.Errorf("mailpit: %w", err)
	}
	// stdlib has no gzip/tar stream unpack for a single member in one
	// shot; system tar handles it on both Linux and macOS.
	stage := fmt.Sprintf("%s-%d-stage", archive, os.Getpid())
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if out, err := exec.Command("tar", "xzf", archive, "-C", stage).CombinedOutput(); err != nil {
		return "", fmt.Errorf("extract mailpit: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.Rename(filepath.Join(stage, "mailpit"), dest); err != nil {
		return "", err
	}
	_ = os.Remove(archive)
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", err
	}
	return dest, nil
}

func pickAsset(m map[string]asset) (asset, error) {
	key := runtime.GOOS + "-" + runtime.GOARCH
	if a, ok := m[key]; ok {
		return a, nil
	}
	return asset{}, fmt.Errorf("no pinned asset for %s (supported: linux-amd64, darwin-arm64; Windows via WSL)", key)
}

// IsVerified reports whether dest exists and matches wantSHA.
func IsVerified(dest, wantSHA string) bool { return isVerified(dest, wantSHA) }

// DownloadVerified exposes verified download for other packages (mariadb, mysql).
func DownloadVerified(dest, url, wantSHA string) error {
	return downloadVerified(dest, url, wantSHA)
}

// DownloadBottle fetches a Homebrew bottle blob from ghcr.io. The
// registry answers anonymous pulls only after a token scoped to the
// formula, so the token dance happens here.
func DownloadBottle(dest, url, wantSHA, formula string) error {
	tokReq, err := http.NewRequest("GET",
		"https://ghcr.io/token?scope=repository:homebrew/core/"+formula+":pull", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(tokReq)
	if err != nil {
		return err
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		resp.Body.Close()
		return err
	}
	resp.Body.Close()

	// The blob URL needs the token as a header; downloadVerified takes
	// none, so stream here with the same tmp+sha discipline.
	tmp := fmt.Sprintf("%s.%d.tmp", dest, os.Getpid())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		out.Close()
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	blob, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		out.Close()
		return err
	}
	defer blob.Body.Close()
	if blob.StatusCode != http.StatusOK {
		out.Close()
		return fmt.Errorf("download %s: HTTP %d", url, blob.StatusCode)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), blob.Body); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, wantSHA)
	}
	return os.Rename(tmp, dest)
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
	// The .tmp suffix carries the pid so two concurrent downloads never
	// write the same file.
	tmp := fmt.Sprintf("%s.%d.tmp", dest, os.Getpid())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	// These assets are 50-200 MB; DefaultClient has no timeout and would
	// hang forever on a stalled connection.
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
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
