# Changelog

## Unreleased

### Added

- Reverse-proxy mode for Node, Ruby, Python, Go, and other app servers: `lserve up --proxy 3000`
- Interactive project detection on a TTY, with a visible `lserve.json` project config shared by interactive and scripted flows
- WebSocket and hot-reload support through the proxy, plus a useful 502 response when the app is not listening
- Upstream status in `lserve list`, plus TUI actions to check and change proxy ports
- Repeatable HTTP and WebSocket e2e test: `test/e2e-proxy.sh ./lserve`
- `lserve up` in any directory without a `./sites` folder now serves the directory itself as `<dirname>.localhost`; no project layout required
- `lserve up --dir <sub>` serves a subfolder as the docroot (`lserve restart --dir <sub>` changes it later)
- macOS: no longer needs a system MySQL or MariaDB. The server (MariaDB 13.0 via pinned Homebrew bottles, dependencies included) downloads automatically on first run, same as Linux. Verified end to end on macOS arm64: MariaDB bootstrap runs directly, no shell in the chain (SIP strips library env there)

### Fixed

- Repeating an unchanged `lserve up` reports `already serving` and skips the daemon reload
- PHP and static projects with frontend markers stay on the direct serving path, including `public/`, `web/`, and `--dir` docroots
- Proxy config and registry changes roll back together after discovery, collision, or reload failures
- Prompt EOF and invalid answers no longer create configuration
- Reserved proxy ports follow environment overrides; Go projects default to a valid port
- Proxy discovery refuses to serve `$HOME` or `/`; non-502 upstream errors keep their real status
- Fresh installs use the publisher's current release digests for FrankenPHP 1.13.4

## 0.1.0 - 2026-10-29

First release. Valet-style PHP development without Docker.

- `lserve up` serves every folder in `./sites` as `https://<name>.localhost:8443` from a shared background frankenphp daemon; the terminal stays free
- Multiple projects at once on one daemon; site name collisions across projects are refused with a named error
- WordPress detected automatically; `WP_HOME`/`WP_SITEURL` injected so canonical redirects do not loop
- Per-user pinned MySQL 8.4, one database per project, `lserve db client|import|dump|dsn`
- `lserve mailpit` runs a pinned Mailpit for local email capture (UI 127.0.0.1:8025, SMTP 127.0.0.1:1025)
- Interactive TUI pickers on a TTY (`lserve list`, `lserve mailpit`), plain tables when piped
- Rootless defaults: HTTP 8080, HTTPS 8443, all loopback; 80/443 via env plus setcap
- Graceful `frankenphp reload` on config changes; registry locked against concurrent edits
- Port conflicts reported as port conflicts, with the fix spelled out
- Platforms: linux-amd64, darwin-arm64 (Windows via WSL)
- `curl | bash` installer into `~/.local/bin`, re-runnable for updates
