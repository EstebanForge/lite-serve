# Plan: reverse-proxy mode for any language

Status: implemented.

## Goal

Serve Node, Ruby, Python, Go, or anything that listens on a port, with the same one-command experience as PHP: `lserve up` gives the project a TLS hostname. The app server stays in the developer's terminal, with its own runtime, debugger, logs, and hot reload.

## Non-goal

Embedding other runtimes. FrankenPHP is unique; there is no equivalent for Ruby/Python/Node/Go. NGINX Unit is archived and needs host libraries. Do not manage app processes in v1.

## UX principles

1. Interactive first. On a TTY (stdin AND stdout), ask with sensible defaults.
2. Non-interactive fails loudly. Piped runs on a non-PHP project without flags print the exact command to use; they never silently serve a wrong site.
3. One config file, visible and committable: `lserve.json`. Written by the prompt, read at discovery.
4. Same mental model as PHP: the directory is the site, the folder name is the host.

## User flows

### Flow A: interactive (TTY)

```
$ lserve up
This looks like a Node.js project (package.json found).
Proxy to a port? [Y/n]: y
Port [3000]:
Wrote lserve.json
Serving https://myapp.localhost:8443 -> 127.0.0.1:3000
Start your app with: npm run dev
```

- Detect project type from marker files and suggest the typical port.
- PHP wins on mixed trees (composer.json or wp-load.php + package.json): no prompt, current behavior.
- The prompt writes `lserve.json`, so the next `up` reads it and never asks again. Choosing "no" writes `{ "mode": "php" }`.
- `--dir` and `--proxy` together are an error.

### Flow B: non-interactive (scripted / CI)

```bash
lserve up --proxy 3000
lserve up --proxy 3000 --name api        # optional host override
```

- `--proxy <port>` registers and writes `lserve.json` too, so later bare `up` works.
- No flags, no `lserve.json`, detected non-PHP project: exit 1 with `looks like Node.js; run: lserve up --proxy 3000`. Never fall back to serving a wrong site.

### Flow C: zero-config by convention

If `lserve.json` exists in the project root, use it. No prompt, no flags.

```json
{ "mode": "proxy", "port": 3000 }
```

The interactive prompt writes this file (and says it did), so the choice is visible, committable, and shared with the team.

### Flow D: list / manage

`lserve list` shows proxy sites with their target:

```
myapp    https://myapp.localhost:8443 -> 127.0.0.1:3000   (node)
blog     https://blog.localhost:8443                      (php)
```

TUI: same picker, plus a "check port" action that reports whether the target is listening.

## Detection defaults

| Marker | Type | Default port | Start hint |
|---|---|---|---|
| `package.json` | node | 3000 | `npm run dev` |
| `Gemfile` | ruby | 3000 | `bundle exec rails s` |
| `pyproject.toml` / `requirements.txt` | python | 8000 | `uvicorn main:app --reload` |
| `go.mod` | go | 8080 | `go run .` |
| `wp-load.php` / `composer.json` / `*.php` | php | - | - |

Detection is a hint only. PHP markers always win; the prompt and `--proxy` flag override the rest.

## Config model

Two layers, one source of truth each:

- **Project config** `lserve.json` in the repo: mode, port, name. Shared with the team. Written by the interactive prompt or by hand.
- **Registry** in the state dir: which directories are served. Local only, never in the repo.

`lserve.json`:

```json
{
  "mode": "proxy",
  "port": 3000,
  "name": "myapp"
}
```

- `mode`: `php` (default when file absent), `proxy`.
- `port`: 1-65535, required for proxy mode. Reject lserve's own ports (8443, 8080, 3306, 8025, 2019) to avoid loops.
- `name`: optional host override; defaults to the folder name.

Registry entry stays minimal: `dir`, `docroot` (PHP only). Mode and port are read from the project file at render time, so there is no drift between registry and repo.

## Implementation plan

### 1. Project config

- Read `lserve.json` at discovery time; validate port range and name.
- Interactive prompt writes it after the user confirms.

### 2. Site model

- Add `Mode`, `Target`, `Kind` to `site.Site`.
- `Discover` reads `lserve.json`; proxy projects become one site with a target instead of a docroot.
- `sites/` + proxy mode is an error, not a silent fallback.

### 3. Registry

- Unchanged: dir and docroot only. No mode duplication.

### 4. Caddy render

- Proxy sites emit `reverse_proxy {{.Target}}` instead of `php_server`.
- Same TLS, same bind, same host rules.
- Static sites use `file_server` (already available via FrankenPHP).

### 5. CLI

- `up --proxy <port>`: non-interactive proxy registration.
- Interactive prompt when: TTY, no `--proxy`, project looks non-PHP.
- `up` on an already-registered proxy project reuses stored target, no prompt.
- `restart` re-reads `.port` if present.

### 6. TUI

- Project list shows mode and target.
- Action menu for proxy projects: `check port`, `change port`, `stop`.

### 7. Port check

- On `up`, if target port is not listening, print the start hint but do not fail. The app may start later.
- `list` / TUI can probe on demand.

## Edge cases

- Upstream down: Caddy `handle_errors` renders a friendly page ("nothing on :3000, start with npm run dev") instead of a bare 502.
- IPv6 upstreams (Vite/Next bind ::1): proxy to `localhost:PORT`, probe both stacks.
- WebSockets/HMR: Caddy reverse_proxy handles upgrade; e2e test it.
- `down` on a proxy project only unregisters; nothing is stopped, no app process is ours.
- TUI action is `remove`, not `stop`, for proxy projects.
- A broken project file must not wedge other projects at reload (existing skip-with-warning applies).

## Test plan

- Unit: detection table (incl. mixed PHP+Node), `lserve.json` parsing, port validation (bad range, own ports), Caddy proxy block render.
- E2E: `lserve up --proxy 9099` with `python3 -m http.server 9099` returns 200 over TLS.
- E2E: `lserve.json` zero-config flow.
- E2E: non-interactive non-PHP project without flags exits 1 with the hint, does not hang.
- E2E: interactive prompt on a pty (via `script`) writes `lserve.json`.
- E2E: WebSocket upgrade through the proxy.
- E2E: `sites/` + `--proxy` errors; `--proxy` + `--dir` errors.

## Open questions

- Multiple ports per project (api + web) is out of scope for v1; revisit if needed. If added, `lserve.json` gains a `sites` map without breaking v1 files.

## Decision

Build interactive-first proxy mode with `lserve.json` as the single project config. Non-interactive runs fail loudly with the exact command. No NGINX Unit, no process management, no new runtime downloads, no static mode (PHP mode already serves static files).
