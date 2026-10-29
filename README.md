# lserve (lite-devenv)

Valet-style PHP development environment without Docker: any folder with a `./sites` directory is served as `https://<name>.localhost` by a pinned frankenPHP binary, backed by a per-user MySQL 8.4 and an optional embedded Mailpit. One shared daemon serves every registered project, like `valet park` for the whole machine.

## How it works

- `lserve up` in a project registers it. Every folder in `./sites` becomes `https://<name>.localhost`. The daemon detaches: the terminal is never locked.
- `lserve up` from any other folder adds that project too; all projects share port 443. Two projects with the same site name are refused with a collision error.
- Docroot resolution per site, first match: `public/`, `web/`, the directory itself (with `wp-load.php` or `index.php`).
- WordPress sites get `WP_HOME`/`WP_SITEURL` injected inside `php_server`, default port omitted so `redirect_canonical` does not loop.
- TLS is Caddy's internal CA: self-signed, browser warning on first visit per site.
- MySQL binds loopback only, root with empty password (dev-only, from `--initialize-insecure`). One database per project, named after the project directory. In `wp-config.php` set `DB_HOST` to `127.0.0.1:3306`, not `localhost` (that would look for the system socket).

## Install and use

```bash
go build -o lserve ./cmd/lserve   # or: go install ./cmd/lserve

mkdir -p myproject/sites/demo/public
echo '<?php phpinfo();' > myproject/sites/demo/public/index.php
cd myproject

lserve up      # daemon starts in the background, terminal returns
lserve list    # interactive picker on a TTY, table when piped
lserve down    # stop serving this directory
```

`lserve` alone prints help.

## Commands

```bash
lserve up [dir]            serve ./sites of a directory (default: cwd)
lserve down [dir]          remove from the registry, reload the daemon
lserve restart [dir]       re-discover sites and reload
lserve list                running projects; enter on one to restart/stop it
lserve logs [mailpit]      last 50 lines of a log
lserve mailpit             menu (start/stop/restart/status); or lserve mailpit start|stop|restart|status
lserve db client           mysql client into the project database
lserve db import dump.sql
lserve db dump [file]      default: stdout
lserve db dsn              mysql://root@127.0.0.1:3306/<project>
```

When the registry becomes empty, the daemon and MySQL stop.

## Ports

`DEVENV_HTTP_PORT` (80), `DEVENV_HTTPS_PORT` (443), `DEVENV_DB_PORT` (3306), `DEVENV_MAILPIT_UI_PORT` (8025), `DEVENV_MAILPIT_SMTP_PORT` (1025). Ports under 1024 need root or `setcap cap_net_bind_service=+ep` on the frankenPHP binary; use 8080/8443 otherwise.

## State directory

`~/.local/state/lite-devenv/` holds pinned binaries (`bin/`), the MySQL datadir (`mysql-data/`, shared across projects), pidfiles and the Caddyfile (`run/`), logs (`log/`), and `registry.json` (served projects). Override with `DEVENV_HOME`.

## OS support

| OS/arch | frankenPHP | Mailpit | MySQL |
|---|---|---|---|
| Linux amd64 | pinned v1.13.4, sha256 | pinned v1.32.0, sha256 | pinned tarball 8.4.5 minimal, sha256 |
| macOS arm64 | pinned v1.13.4, sha256 | pinned v1.32.0, sha256 | system `mysqld` from PATH (`brew install mysql`) |

Windows runs under WSL (linux-amd64). Other platforms fail visibly.

## Not goals

- Production use. Loopback binds, empty root password, internal CA: all dev-only.
- PHP version switching (one pinned frankenPHP per release).
- Graceful Caddy reloads (daemon restarts on config change; dev traffic tolerates the gap).
