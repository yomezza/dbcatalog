#!/usr/bin/env bash
# Run inside a fresh D-Bus session and X display (e.g. dbus-run-session -- xvfb-run -a ...).
set -euo pipefail
cd "$(dirname "$0")/.."
command -v xdotool >/dev/null
log=${CATALOGDB_LINUX_SMOKE_LOG:-build/linux-launch.log}
mkdir -p "$(dirname "$log")"
build/bin/CatalogDB > "$log" 2>&1 &
app_pid=$!
trap 'kill "$app_pid" 2>/dev/null || true; wait "$app_pid" 2>/dev/null || true' EXIT
for attempt in $(seq 1 75); do
 if ! kill -0 "$app_pid" 2>/dev/null; then
  cat "$log"
  echo 'CatalogDB exited before creating a window' >&2
  exit 1
 fi
 if window=$(xdotool search --onlyvisible --name 'CatalogDB' 2>/dev/null | head -n 1) && [[ -n "$window" ]]; then
  xdotool getwindowname "$window"
  sleep 2
  kill -0 "$app_pid"
  echo 'Linux desktop window is visible and the application remains running.'
  exit 0
 fi
 sleep 0.2
done
cat "$log"
echo 'No visible CatalogDB window appeared' >&2
exit 1
