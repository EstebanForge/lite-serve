# lite-devenv

Valet-style PHP development environment without Docker: parked folders under `./sites` served as `https://<name>.localhost` by a pinned frankenPHP binary, plus a per-user MySQL 8.4 (pinned tarball on Linux amd64, system `mysqld` on macOS).

## How it works

- `sites/<name>/` becomes `https://<name>.localhost`. `*.localhost` resolves to 127.0.0.1 on macOS and systemd-resolved Linux; no `/etc/hosts` edits.
- Docroot resolution, first match: `public/`, `web/`, directory root (with `wp-load.php` or `index.php`).
- WordPress sites (docroot has `wp-load.php`) get `WP_HOME`/`WP_SITEURL` injected inside `php_server`, with the default port omitted so `redirect_canonical` does not loop.
- TLS is Caddy's internal CA: self-signed, browser warning on first visit per site.
- MySQL binds loopback only, root with empty password (dev-only, from `--initialize-insecure`). One database per project, named after the project directory. WP needs `DB_HOST` set to `127.0.0.1:<port>`, not `localhost` (that would look for the system socket).

## Usage

```bash
go build -o devenv ./cmd/devenv   # or: go install ./cmd/devenv

mkdir -p myproject/sites/demo/public
echo '<?php phpinfo();' > myproject/sites/demo/public/index.php
cd myproject

devenv start     # boots MySQL (first run: downloads ~70 MB) and frankenPHP
devenv status
devenv stop
```

Ports: `DEVENV_HTTP_PORT` (80), `DEVENV_HTTPS_PORT` (443), `DEVENV_DB_PORT` (3306). Ports under 1024 need root or `setcap cap_net_bind_service=+ep` on the frankenPHP binary; use 8080/8443 otherwise.

## Database

```bash
devenv db client                 # mysql client, connected to the project DB
devenv db client -e 'SHOW TABLES;'
devenv db import dump.sql
devenv db dump [file]            # default: stdout
devenv db dsn                    # mysql://root@127.0.0.1:3306/<project>
```

## State directory

`~/.local/state/lite-devenv/` holds downloaded binaries (`bin/`, `mysql/`), the MySQL datadir (`mysql-data/`, shared across projects), pidfiles and the Caddyfile (`run/`), and logs (`log/`). Override with `DEVENV_HOME`.

## OS support

| OS/arch | frankenPHP | MySQL |
|---|---|---|
| Linux amd64 | pinned binary v1.13.4, sha256-verified | pinned tarball 8.4.5 minimal, sha256-verified |
| macOS arm64 | pinned binary v1.13.4, sha256-verified | system `mysqld` from PATH (`brew install mysql`) |

Other platforms fail visibly with a clear message instead of half-starting.

## Not goals

- Production use. Loopback bind, empty root password, internal CA: all dev-only.
- PHP version switching (one pinned frankenPHP per release, like the Wicket lite path).
- nginx/apache parity, Mailpit, Xdebug wiring (may come later).
