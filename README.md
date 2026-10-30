# lite-serve

Valet-style PHP development without Docker. One command serves every project on your machine as `https://<folder>.localhost`, with MySQL and Mailpit included. I built it because spinning up containers for a personal WP site is overkill.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/EstebanForge/lite-serve/main/install.sh | bash
```

Puts `lserve` in `~/.local/bin`. Run it again any time to update; it overwrites the existing binary. If there is no release yet it builds from source, so you need Go for that path.

## 60 seconds to a running site

```bash
mkdir myproject && cd myproject
echo '<?php phpinfo();' > index.php
lserve up
# open https://myproject.localhost:8443
```

That is the whole model. Two ways to serve:

- Any directory: the folder itself becomes `<dirname>.localhost`. One site, no setup. `lserve up --dir wp` serves a subfolder (`wp/`) as the docroot instead; `lserve restart --dir wp` changes it later.
- `./sites/<name>` inside a project: every folder becomes `<name>.localhost`. Several sites per project.

WordPress is detected automatically and gets `WP_HOME`/`WP_SITEURL` injected, so no redirect loops.

Node, Ruby, Python, Go, and other app servers use proxy mode. Run your app as usual; lite-serve gives it HTTPS and a hostname:

```bash
lserve up --proxy 3000
# https://myproject.localhost:8443 -> localhost:3000
npm run dev
```

On a TTY, `lserve up` detects common app projects and asks for the port. Both paths write a visible, committable `lserve.json`:

```json
{
  "mode": "proxy",
  "port": 3000
}
```

After that, bare `lserve up` needs no flags. Scripts without flags stop with the exact proxy command instead of serving the wrong site. WebSockets and hot reload work through the proxy. If the app is down, the site explains how to start it. `lserve list` shows whether each upstream is listening; the interactive menu can check or change its port.

## Commands

```bash
lserve up [dir]     serve this directory (background daemon, terminal stays free)
lserve up --proxy 3000  HTTPS hostname for an app already listening on port 3000
lserve down         stop serving it
lserve list         interactive picker on a TTY, table when piped
lserve logs         tail the server log
lserve mailpit      local email capture, UI on 127.0.0.1:8025
lserve db client    mysql shell into the project database
```

Multiple projects run at once on one shared daemon. `lserve down` on the last one stops everything.

Sites survive terminal closure but stop at logout. A per-login watcher uses logind on Linux and the user launchd manager on macOS, then stops FrankenPHP, MySQL, and Mailpit. The project registry remains, so the next `lserve up` restores the sites. Nothing starts at boot.

## Under the hood

- Pinned frankenPHP serves all sites. TLS is a local CA, so the browser warns once per site. This is normal.
- Pinned MySQL 8.4 (Linux) or MariaDB (macOS) per user, downloaded automatically on first run, one database per project, root with no password. Dev only, loopback only. Never touches a database server installed on your system.
- Pinned Mailpit for email. Start it when you need it.
- Linux (amd64) and macOS (arm64). Windows runs fine under WSL. Anything else fails loudly on purpose.
- Everything lives in `~/.local/state/lite-serve/`. Delete it to uninstall completely.
- Defaults are rootless: HTTP 8080, HTTPS 8443. Want `https://demo.localhost` without the port? Set `DEVENV_HTTPS_PORT=443` and give frankenPHP the bind capability:

```bash
sudo setcap cap_net_bind_service=+ep ~/.local/state/lite-serve/bin/frankenphp
```

## Not goals

Production use, PHP version switching, graceful-everything. This is a dev tool that stays out of your way.

## Develop

```bash
go build -o lserve ./cmd/lserve
go test ./...
```

No Docker. No required config files. No daemons named after Greek gods.
