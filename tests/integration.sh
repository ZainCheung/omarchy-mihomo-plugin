#!/usr/bin/env bash
set -euo pipefail

# Command-level integration tests for the manager. Everything runs against a
# temporary store, fake controller and fake Mihomo binary; no systemd service
# or real controller is touched.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'kill "${HTTP_PID:-}" 2>/dev/null || true; rm -rf "$TMP"' EXIT

STORE="$TMP/store"
FAKE="$TMP/fake"
mkdir -p "$FAKE/http"
export OMARCHY_MIHOMO_HOME="$STORE"
export OMARCHY_MIHOMO_ALLOW_PRIVATE_HOSTS=1
export FAKE_LIVE_CONFIG="$FAKE/live.yaml"
export FAKE_CONFIG="$FAKE/config.yaml"
export FAKE_FAIL_CONFIG_ONCE="$FAKE/fail-config-once"

cat >"$FAKE/config.yaml" <<'YAML'
port: 7890
mode: rule
log-level: warn
external-controller: 127.0.0.1:9090
secret: test-secret
YAML
cp "$FAKE/config.yaml" "$FAKE/live.yaml"

cat >"$FAKE/http/a.yaml" <<'YAML'
port: 7890
mode: rule
log-level: info
proxies:
  - name: DIRECT
    type: direct
proxy-groups:
  - name: Select
    type: select
    proxies: [DIRECT]
YAML
cat >"$FAKE/http/b.yaml" <<'YAML'
port: 7890
mode: rule
log-level: debug
proxies:
  - name: DIRECT
    type: direct
proxy-groups:
  - name: Select
    type: select
    proxies: [DIRECT]
YAML
cat >"$FAKE/http/etag.yaml" <<'YAML'
port: 7890
mode: rule
log-level: error
proxies:
  - name: DIRECT
    type: direct
proxy-groups:
  - name: Select
    type: select
    proxies: [DIRECT]
YAML

cat >"$FAKE/http/server.py" <<'PY'
import http.server
import os
import pathlib
import socketserver

root = pathlib.Path(os.environ["HTTP_ROOT"])
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        route = self.path.split("?", 1)[0]
        if route == "/a":
            name, tag = "a.yaml", None
        elif route == "/b":
            name, tag = "b.yaml", None
        elif route == "/etag":
            name, tag = "etag.yaml", "etag-v1"
        elif route == "/bad":
            body = b"not: [valid\n"
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        else:
            self.send_error(404)
            return
        if tag and self.headers.get("If-None-Match") == tag:
            self.send_response(304)
            self.send_header("ETag", tag)
            self.send_header("Subscription-Userinfo", "upload=10; download=20; total=100; expire=1700000000")
            self.end_headers()
            return
        body = (root / name).read_bytes()
        self.send_response(200)
        if tag:
            self.send_header("ETag", tag)
        self.send_header("Content-Type", "text/yaml")
        self.send_header("Subscription-Userinfo", "upload=10; download=20; total=100; expire=1700000000")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *_):
        pass

with socketserver.TCPServer(("127.0.0.1", 0), Handler) as server:
    pathlib.Path(os.environ["HTTP_PORT_FILE"]).write_text(str(server.server_address[1]))
    server.serve_forever()
PY
HTTP_ROOT="$FAKE/http" HTTP_PORT_FILE="$FAKE/port" python3 "$FAKE/http/server.py" &
HTTP_PID=$!
for _ in $(seq 1 50); do [[ -s "$FAKE/port" ]] && break; sleep 0.1; done
PORT="$(cat "$FAKE/port")"

cat >"$FAKE/mihomo-ctl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-} ${2:-}" in
  "coreinfo ")
    printf '{"pid":123,"exe":"%s","configPath":"%s","configDir":"%s","controllerTransport":"tcp","controllerTarget":"127.0.0.1:9090"}\n' "$FAKE_MIHOMO_BIN" "$FAKE_CONFIG" "$(dirname "$FAKE_CONFIG")"
    ;;
  "get /version")
    printf '{"meta":true,"version":"fake"}\n'
    ;;
  "get /configs")
    if [[ "${FAKE_FAIL_CONFIG_CHECK_ONCE:-0}" == 1 && ! -e "$FAKE_FAIL_CONFIG_ONCE" ]]; then
      : >"$FAKE_FAIL_CONFIG_ONCE"
      printf 'simulated controller config-check failure\n' >&2
      exit 1
    fi
    printf '{"port":7890,"mode":"rule"}\n'
    ;;
  "put-file /configs?force=true")
    body="$(cat "${3:?body file required}")"
    source_path="$(python3 -c 'import json,sys; x=json.loads(sys.argv[1]); print(x.get("path", ""))' "$body")"
    if [[ -n "$source_path" ]]; then
      cp "$source_path" "$FAKE_LIVE_CONFIG"
    else
      python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("payload", ""), end="")' "$body" >"$FAKE_LIVE_CONFIG"
    fi
    printf '{"ok":true}\n'
    ;;
  "put /configs?force=true")
    body="${3:-}"
    source_path="$(python3 -c 'import json,sys; x=json.loads(sys.argv[1]); print(x.get("path", ""))' "$body")"
    if [[ -n "$source_path" ]]; then
      cp "$source_path" "$FAKE_LIVE_CONFIG"
    else
      python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("payload", ""), end="")' "$body" >"$FAKE_LIVE_CONFIG"
    fi
    printf '{"ok":true}\n'
    ;;
  *)
    printf 'unsupported fake ctl command: %s\n' "$*" >&2
    exit 2
    ;;
esac
SH
chmod +x "$FAKE/mihomo-ctl"

cat >"$FAKE/mihomo" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_VALIDATE_FAIL:-0}" == 1 ]]; then
  echo 'simulated Mihomo validation failure' >&2
  exit 1
fi
[[ "${1:-}" == "-t" && "${2:-}" == "-f" ]] || { echo 'bad validator args' >&2; exit 2; }
[[ -s "$3" ]] || { echo 'empty config' >&2; exit 1; }
grep -q '^port:' "$3" || { echo 'missing port' >&2; exit 1; }
exit 0
SH
chmod +x "$FAKE/mihomo"

GO_BIN="${GO_BIN:-$(command -v go || true)}"
[[ -n "$GO_BIN" ]] || { echo 'go is required for integration tests' >&2; exit 1; }
"$GO_BIN" -C "$ROOT/manager" build -trimpath -o "$TMP/manager" ./cmd/omarchy-mihomo-manager
export MIHOMO_CTL="$FAKE/mihomo-ctl"
export MIHOMO_BIN="$FAKE/mihomo"
export FAKE_MIHOMO_BIN="$FAKE/mihomo"
MANAGER="$TMP/manager"

json_value() { python3 -c 'import json,sys; x=json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"; }
get_id() { json_value 'x["data"]["id"]'; }
fail_cmd() { if "$MANAGER" "$@" >"$TMP/fail.out" 2>"$TMP/fail.err"; then echo "expected failure: $*" >&2; exit 1; fi; }
assert_eq() { [[ "$1" == "$2" ]] || { echo "assertion failed: '$1' != '$2'" >&2; exit 1; }; }
assert_file_contains() { grep -Fq "$2" "$1" || { echo "missing '$2' in $1" >&2; exit 1; }; }

A_URL="http://127.0.0.1:$PORT/a"
B_URL="http://127.0.0.1:$PORT/b"
ETAG_URL="http://127.0.0.1:$PORT/etag"
SECRET_URL="http://127.0.0.1:$PORT/a?token=integration-secret"

A_JSON="$($MANAGER profile add --url "$A_URL" --name A)"
A_ID="$(printf '%s' "$A_JSON" | get_id)"
[[ "$A_JSON" == *'"download":20'* ]] || { echo 'subscription quota was not persisted' >&2; exit 1; }
B_JSON="$($MANAGER profile add --url "$B_URL" --name B)"
B_ID="$(printf '%s' "$B_JSON" | get_id)"
ETAG_JSON="$($MANAGER profile add --url "$ETAG_URL" --name ETag)"
ETAG_ID="$(printf '%s' "$ETAG_JSON" | get_id)"
SECRET_JSON="$($MANAGER profile add --url "$SECRET_URL" --name Secret)"
SECRET_ID="$(printf '%s' "$SECRET_JSON" | get_id)"
[[ "$SECRET_JSON" != *integration-secret* ]] || { echo 'secret leaked in add output' >&2; exit 1; }
STATUS="$($MANAGER status)"
[[ "$STATUS" != *integration-secret* ]] || { echo 'secret leaked in status output' >&2; exit 1; }
RAW_URL="$($MANAGER profile url "$SECRET_ID")"
[[ "$RAW_URL" == *integration-secret* ]] || { echo 'explicit URL read did not return original URL' >&2; exit 1; }

$MANAGER profile select "$A_ID" >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: info'
assert_file_contains "$STORE/runtime/state.json" '"secret": "test-secret"'
$MANAGER profile select "$B_ID" >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: debug'
$MANAGER profile select "$A_ID" >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: info'
$MANAGER profile select "$B_ID" >/dev/null
$MANAGER config rollback >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: info'
assert_file_contains "$STORE/profiles/index.json" "$A_ID"
$MANAGER config rollback >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: debug'
assert_file_contains "$STORE/profiles/index.json" "$B_ID"
$MANAGER profile select "$A_ID" >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: info'

fail_cmd profile delete "$A_ID"
assert_file_contains "$TMP/fail.err" 'cannot delete the active profile'
assert_file_contains "$STORE/profiles/index.json" "$A_ID"

# A bad subscription must not create a profile or replace any source.
fail_cmd profile add --url "http://127.0.0.1:$PORT/bad" --name Bad
assert_file_contains "$TMP/fail.err" 'parse'
[[ "$(find "$STORE/profiles" -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 4 ]] || { echo 'bad profile was persisted' >&2; exit 1; }

# An inactive update must compile and validate before replacing its source.
cp "$STORE/profiles/$B_ID/source.yaml" "$TMP/b-before.yaml"
export FAKE_VALIDATE_FAIL=1
fail_cmd profile update "$B_ID"
unset FAKE_VALIDATE_FAIL
assert_file_contains "$TMP/fail.err" 'simulated Mihomo validation failure'
cmp "$STORE/profiles/$B_ID/source.yaml" "$TMP/b-before.yaml"

# Conditional request path: the second fetch must be HTTP 304.
$MANAGER profile update "$ETAG_ID" >/dev/null
ETAG_RESULT="$($MANAGER profile update "$ETAG_ID")"
[[ "$ETAG_RESULT" == *'"notModified":true'* ]] || { echo 'ETag 304 was not handled' >&2; exit 1; }

# Controller failure after applying the candidate must roll back controller,
# current runtime, previous/candidate files and active index state.
cp "$STORE/runtime/current.yaml" "$TMP/current-a.yaml"
export FAKE_FAIL_CONFIG_CHECK_ONCE=1
fail_cmd profile select "$B_ID"
unset FAKE_FAIL_CONFIG_CHECK_ONCE
cmp "$STORE/runtime/current.yaml" "$TMP/current-a.yaml"
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: info'
assert_file_contains "$STORE/profiles/index.json" "$A_ID"
assert_file_contains "$STORE/runtime/state.json" "$A_ID"

# Reconcile restores the active profile after the core's live config changes.
cp "$FAKE/config.yaml" "$FAKE/live.yaml"
$MANAGER reconcile >/dev/null
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: info'

# Exercise the current.yaml-missing fallback during a failed apply.
rm "$STORE/runtime/current.yaml"
cp "$FAKE/config.yaml" "$FAKE/live.yaml"
: >"$FAKE_FAIL_CONFIG_ONCE"; rm -f "$FAKE_FAIL_CONFIG_ONCE"
export FAKE_FAIL_CONFIG_CHECK_ONCE=1
fail_cmd profile select "$B_ID"
unset FAKE_FAIL_CONFIG_CHECK_ONCE
[[ ! -e "$STORE/runtime/current.yaml" ]] || { echo 'current.yaml was recreated after missing-current rollback' >&2; exit 1; }
assert_file_contains "$FAKE_LIVE_CONFIG" 'log-level: warn'
assert_file_contains "$STORE/profiles/index.json" "$A_ID"
assert_file_contains "$STORE/runtime/state.json" "$A_ID"

# Settings validation and a successful active apply.
$MANAGER settings set dns-management inherit >/dev/null
assert_file_contains "$STORE/settings.json" '"dnsManagement": "inherit"'
$MANAGER settings set dns-enable false >/dev/null
assert_file_contains "$STORE/settings.json" '"enable": false'
$MANAGER settings set tun-stack system >/dev/null
assert_file_contains "$STORE/settings.json" '"stack": "system"'
$MANAGER settings patch dns-enable true dns-ipv6 true tun-stack mixed >/dev/null
assert_file_contains "$STORE/settings.json" '"enable": true'
assert_file_contains "$STORE/settings.json" '"ipv6": true'
assert_file_contains "$STORE/settings.json" '"stack": "mixed"'
$MANAGER settings set dns-nameserver '1.1.1.1, 8.8.8.8' >/dev/null
assert_file_contains "$STORE/settings.json" '8.8.8.8'
fail_cmd settings set tun-management invalid
assert_file_contains "$TMP/fail.err" 'managed or inherit'

printf 'manager integration tests: PASS\n'
