# Changelog

## 0.1.0 - 2026-10-29

First release. Valet-style local development for PHP and application servers, without Docker.

### Added

- `lserve up` serves the current directory as `https://<dirname>.localhost:8443`; no project layout or configuration file is required
- Projects with `./sites` serve every child folder as `https://<name>.localhost:8443`; hidden folders are ignored, symlinks are followed, and invalid hostnames are rejected
- Docroots resolve through `public/`, `web/`, or the project root; static sites work directly; `--dir <sub>` selects a subfolder and `restart --dir <sub>` changes it later
- WordPress is detected automatically, with `WP_HOME` and `WP_SITEURL` injected to prevent canonical redirect loops
- Multiple projects share one detached FrankenPHP daemon; terminals stay free, hostname collisions are rejected, and registry updates are locked against concurrent edits
- Reverse-proxy mode for Node, Ruby, Python, Go, and other application servers through `lserve up --proxy <port>` and optional `--name <hostname>`
- Interactive runtime detection and proxy setup on a TTY; non-interactive detection prints the required command instead of guessing
- Visible, committable `lserve.json` project configuration for PHP or proxy mode, ports, and proxy hostnames
- WebSocket and hot-reload proxying, upstream status in `lserve list`, a useful 502 response when the application is not listening, and TUI actions to check or change proxy ports
- One pinned MySQL 8.4 server on Linux and MariaDB 13.0 with bundled dependencies on macOS, downloaded automatically on first use; no host database server is used
- One database per PHP project, with `lserve db client`, `import`, `dump`, and `dsn`; failed dumps preserve existing files
- Pinned Mailpit for local email capture, with interactive and non-interactive start, stop, restart, and status commands; UI defaults to `127.0.0.1:8025` and SMTP to `127.0.0.1:1025`
- Interactive project and Mailpit TUIs on a TTY, with plain tables and scriptable commands when piped
- Rootless HTTP 8080 and HTTPS 8443 defaults, loopback-only services, environment overrides for every port, and documented opt-in 80/443 operation
- Local TLS through the FrankenPHP internal CA
- Graceful FrankenPHP reloads, atomic Caddyfile writes, HTTPS health checks, and explicit port-conflict diagnostics
- Idempotent repeated `up`: unchanged projects report `already serving`, skip unnecessary reloads, repair MySQL, and verify HTTPS health
- Login-session ownership for FrankenPHP, the database, and Mailpit: sites survive terminal closure but stop at logout; the project registry remains for the next `up`
- Linux logind and macOS launchd session tracking, including closing-session detection, background-online session support, transient lookup tolerance, multi-login ownership, and serialized watcher startup
- Read-only `lserve doctor` for platform, build, state, embedded binaries, localhost DNS, ports, daemon and watcher health, registry projects, and proxy upstreams; warnings cover optional or unused services and failures exit nonzero
- `lserve version`, `--version`, and `-v` with a build timestamp
- `curl | bash` installation into `~/.local/bin`, with release downloads and update support
- Hello World examples for PHP, Node, Ruby, Python, and Go under `examples/`, including run instructions and proxy configuration
- Repeatable HTTP, upstream-error, and WebSocket end-to-end coverage through `test/e2e-proxy.sh`
- Platforms: linux-amd64 and darwin-arm64; Windows through WSL

### Fixed

- PHP and static projects with frontend markers stay on the direct serving path, including `public/`, `web/`, and `--dir` docroots
- Proxy configuration and registry changes roll back together after validation, collision, or reload failures; configuration writes are atomic and preserve file modes
- Prompt EOF and invalid answers do not create configuration; reserved proxy ports follow environment overrides, and Go projects use a non-conflicting default
- Proxy discovery refuses unsafe project roots, and non-502 upstream errors retain their actual status
- Process lifecycle handles stale pidfiles, zombies, PID reuse, exact executable identity, termination escalation, and concurrent daemon startup
- Final `down` stops FrankenPHP, the database, Mailpit, and the session watcher
- Session support is checked before project or daemon mutations, and every daemon-start path restores watcher coverage
- Fresh installs use verified publisher digests for pinned FrankenPHP assets
