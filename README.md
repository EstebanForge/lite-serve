# lite-serve

Valet-style PHP development without Docker. One command serves every project on your machine as `https://<folder>.localhost`, with MySQL and Mailpit included. I built it because spinning up containers for a personal WP site is overkill.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/EstebanForge/lite-serve/main/install.sh | bash
```

Puts `lserve` in `~/.local/bin`. Run it again any time to update; it overwrites the existing binary. If there is no release yet it builds from source, so you need Go for that path.

## 60 seconds to a running site

```bash
mkdir -p myproject/sites/demo/public
echo '<?php phpinfo();' > myproject/sites/demo/public/index.php
cd myproject

lserve up
# open https://demo.localhost:8443
```

That is the whole model. Every folder in `./sites` becomes a hostname. WordPress is detected automatically and gets `WP_HOME`/`WP_SITEURL` injected, so no redirect loops.

## Commands

```bash
lserve up [dir]     serve this directory's sites (background daemon, terminal stays free)
lserve down         stop serving it
lserve list         interactive picker on a TTY, table when piped
lserve logs         tail the server log
lserve mailpit      local email capture, UI on 127.0.0.1:8025
lserve db client    mysql shell into the project database
```

Multiple projects run at once on one shared daemon. `lserve down` on the last one stops everything.

## Under the hood

- Pinned frankenPHP serves all sites. TLS is a local CA, so the browser warns once per site. This is normal.
- Pinned MySQL 8.4 per user, one database per project, root with no password. Dev only, loopback only.
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

No Docker. No config files. No daemons named after Greek gods.
