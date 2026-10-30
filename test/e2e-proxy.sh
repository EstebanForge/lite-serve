#!/usr/bin/env bash
set -euo pipefail

# Run after `go build -o lserve ./cmd/lserve`: test/e2e-proxy.sh ./lserve
LSERVE_ARG=${1:-./lserve}
LSERVE=$(cd "$(dirname "$LSERVE_ARG")" && pwd)/$(basename "$LSERVE_ARG")
TMP=$(mktemp -d)
UPSTREAM_PID=""
cleanup() {
  [[ -z "$UPSTREAM_PID" ]] || kill "$UPSTREAM_PID" 2>/dev/null || true
  DEVENV_HOME="$TMP/state" DEVENV_HTTPS_PORT=18443 DEVENV_HTTP_PORT=18080 \
    "$LSERVE" down "$TMP/app" >/dev/null 2>&1 || true
  rm -r -- "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/app"
printf '{"scripts":{"dev":"python3 -m http.server 19100"}}\n' >"$TMP/app/package.json"
printf 'proxy e2e ok\n' >"$TMP/app/index.html"

export DEVENV_HOME="$TMP/state"
export DEVENV_HTTPS_PORT=18443
export DEVENV_HTTP_PORT=18080

"$LSERVE" up "$TMP/app" --proxy 19100 >/dev/null
again=$("$LSERVE" up "$TMP/app" --proxy 19100)
printf '%s' "$again" | grep -q 'already serving'
curl -sSk https://app.localhost:18443/ | grep -q 'Nothing is listening'
python3 -m http.server 19100 --bind 127.0.0.1 --directory "$TMP/app" >"$TMP/http.log" 2>&1 &
UPSTREAM_PID=$!
sleep 0.5
curl --retry 5 --retry-all-errors --retry-delay 1 -fsSk https://app.localhost:18443/ | grep -q 'proxy e2e ok'
kill "$UPSTREAM_PID"
wait "$UPSTREAM_PID" 2>/dev/null || true
UPSTREAM_PID=""

cat >"$TMP/ws.go" <<'GO'
package main
import("crypto/sha1";"encoding/base64";"fmt";"net/http";"time")
func main(){http.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){c,b,_:=w.(http.Hijacker).Hijack();defer c.Close();sum:=sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key")+"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"));fmt.Fprintf(b,"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",base64.StdEncoding.EncodeToString(sum[:]));b.Flush();time.Sleep(time.Second)});panic(http.ListenAndServe("127.0.0.1:19100",nil))}
GO
go build -o "$TMP/ws" "$TMP/ws.go"
"$TMP/ws" >"$TMP/ws.log" 2>&1 &
UPSTREAM_PID=$!
sleep 0.5
status=$(curl --retry 5 --retry-all-errors --retry-delay 1 --http1.1 -sk -o /dev/null -w '%{http_code}' \
  -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
  -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' \
  https://app.localhost:18443/ || true)
[[ "$status" == "101" ]]
printf 'proxy e2e: HTTP, upstream-down response, and WebSocket passed\n'
