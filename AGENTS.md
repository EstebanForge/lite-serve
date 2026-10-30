# AGENTS.md

Operating guide for coding agents working in this repo.

## Project overview

`lite-serve` is a Go CLI (`lserve`) that runs a Valet-style local PHP dev environment without Docker: one pinned frankenPHP daemon serves every registered project's `./sites/<name>` folders as `https://<name>.localhost`, backed by a per-user MySQL 8.4 and an optional pinned Mailpit. Platforms: linux-amd64 and darwin-arm64 only (Windows via WSL); other platforms must fail visibly.

## Setup

- Prereqs: Go 1.27+ (`go.mod`), `tar` and `xz` on PATH (used to extract binary tarballs), a C compiler is NOT needed.
- Bootstrap: `go mod download`. There is no codegen and no Makefile.

## Commands

All verified; there is no lint or CI config in the repo.

### Quick Commands (`just` recipes)

```bash
just            # List all available targets
just build      # Build binary to bin/lserve
just fmt        # Format Go source code
just vet        # Run go vet
just test       # Run unit tests (-short)
just check      # Run static checks and unit tests
just ci         # Validate and build locally
```

Or via direct Go commands:

```bash
go build -o lserve ./cmd/lserve   # build
go vet ./...                      # static check (the only lint gate)
gofmt -l .                        # must print nothing
go test ./...                     # all tests
go test ./internal/site/ -run TestDiscover  # single package/test
go build -o lserve ./cmd/lserve && test/e2e-proxy.sh ./lserve  # real HTTP + WebSocket proxy e2e
```

Dev-run against a scratch project (tests never touch the real state dir):

```bash
mkdir -p /tmp/proj/sites/demo/public
echo '<?php echo "hi";' > /tmp/proj/sites/demo/public/index.php
cd /tmp/proj && ../lserve up      # daemon detaches; no env vars needed
curl -sk https://demo.localhost:8443/
```

## Architecture

- `cmd/lserve/main.go` — all CLI wiring (`up/down/restart/list/logs/mailpit/db`). The `app` struct owns home dir, registry, binary path, and DB handle. `reload()` renders the shared Caddyfile and reloads or starts the daemon.
- `internal/site` — discovery and optional `lserve.json` project config: with `./sites`, each child becomes `<name>.localhost`; without it the cwd itself becomes one site named by its basename (refuses $HOME and /). Proxy mode serves one `localhost:<port>` upstream instead of a docroot. Docroot = first of `public/`, `web/`, root with `index.php`/`wp-load.php`; WordPress detected by `wp-load.php`. Pure functions, heavily tested.
- `internal/caddy` — renders ONE Caddyfile for all registered projects (sorted output; case-insensitive hostname collision detection), with `php_server` or `reverse_proxy` blocks and friendly upstream errors.
- `internal/registry` — JSON registry of served projects at `~/.local/state/lite-serve/registry.json`; flock-guarded read-modify-write.
- `internal/bin` — pinned binary downloads (frankenPHP v1.13.4, Mailpit v1.32.0) with sha256 verification; system `tar` does the extraction.
- `internal/db` — embedded server, auto-downloaded: pinned MySQL 8.4.5 tarball on linux-amd64; pinned MariaDB 13.0 Homebrew bottles (plus openssl@4 and pcre2 bottles) on darwin-arm64, wired via DYLD_LIBRARY_PATH with no binary patching. Never uses a host-installed server. Datadir is shared under the state dir; one database per project, named after the project directory. Flavor (mysql/mariadb) is detected from `mysqld --version` and drives init and server flags.
- `internal/proc` — pidfile lifecycle for frankenPHP and mailpit: zombie rejection via `ps` stat, process identity check, SIGTERM then SIGKILL.
- `internal/mailpit` — detached Mailpit on 127.0.0.1:8025/1025 with a port-ready health check.
- `internal/tui` — bubbletea pickers (projects, mailpit). Logic stays in `cmd/lserve` via injected callbacks; TUI takes data + callbacks. Piped stdout gets a table instead.

Data flow: `lserve up` -> registry.Add -> discoverAll -> caddy.Render -> atomic Caddyfile write -> `frankenphp reload` (graceful; falls back to restart) -> waitDaemonUp (pid + port + settle check). Empty registry stops daemon and MySQL.

State dir: `~/.local/state/lite-serve/` (`bin/`, `run/`, `log/`, `mysql-data/`, `registry.json`), override with `DEVENV_HOME`. A one-time migration renames a legacy `lite-devenv` state dir.

## Conventions and gotchas

- Only stdlib + charmbracelet deps; keep it that way unless the user asks.
- Defaults are rootless: HTTP 8080, HTTPS 8443, DB 3306 (env: `DEVENV_HTTP_PORT`, `DEVENV_HTTPS_PORT`, `DEVENV_DB_PORT`, `DEVENV_MAILPIT_*_PORT`). Never default to ports under 1024.
- The port health check must verify BOTH our pid alive AND the port answering; a squatter answers the port too (bind failures looked healthy before).
- Never index a pinned-asset map before the platform check (panic on unsupported OS). New binaries need a pin + sha256 per platform in `internal/bin`.
- MySQL connects as root with empty password (dev-only, `--initialize-insecure`); wp-config must use `127.0.0.1:3306`, not `localhost` (socket mismatch).
- TLS is Caddy internal CA; first visit per host shows a browser warning. This is expected.
- `lsof` is not used anywhere (macOS has no `-s` flag); process checks go through `ps`.
- Tests: stdlib `testing` only, colocated `*_test.go`, table-driven where sensible. Test discovery and rendering logic here; process lifecycle is covered by e2e scripts, not unit tests.
- Module path, repo dir, and state dir are all `lite-serve`; only the legacy state dir name `lite-devenv` survives, in `internal/home` migration code.

## Commit guidelines

From git history: Conventional Commits, lowercase, imperative, scope implied (`feat:`, `fix:`), body explains WHY not what. Examples: `feat: valet-style lserve with project registry, TUI menus, and embedded Mailpit`, `fix: lock registry, graceful reloads, and TUI state after second peer review`. No PR/CI requirements documented; repo is local-only.
