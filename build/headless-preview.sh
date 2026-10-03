#!/usr/bin/env bash
set -euo pipefail

repository_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
socket_name="wayland-sshbrowse-preview"

usage() {
  echo "Usage: $0 run | shot <preview-dir> [output.png]" >&2
  exit 2
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing $1. Install Weston and its screenshooter utility for headless previews." >&2
    exit 1
  fi
}

case "${1:-}" in
  run)
    [[ $# -eq 1 ]] || usage
    require_command weston
    require_command weston-screenshooter
    require_command wails3
    umask 077
    runtime_dir="$(mktemp -d /tmp/sshbrowse-preview.XXXXXXXX)"

    weston_pid=""
    app_pid=""
    cleanup() {
      if [[ -n "$app_pid" ]]; then
        kill "$app_pid" 2>/dev/null || true
        wait "$app_pid" 2>/dev/null || true
      fi
      if [[ -n "$weston_pid" ]]; then
        kill "$weston_pid" 2>/dev/null || true
        wait "$weston_pid" 2>/dev/null || true
      fi
      rm -rf -- "$runtime_dir"
    }
    trap cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    mcp_token="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
    if [[ ${#mcp_token} -ne 64 ]]; then
      echo "Could not generate a preview MCP token." >&2
      exit 1
    fi
    printf '%s\n' "$mcp_token" > "$runtime_dir/mcp-token"

    cd "$repository_dir"
    wails3 task build EXTRA_TAGS=mcp OUTPUT="$runtime_dir/sshbrowse"

    XDG_RUNTIME_DIR="$runtime_dir" weston --backend=headless --renderer=pixman --no-config --debug \
      --width=1280 --height=900 --socket="$socket_name" --idle-time=0 \
      --log="$runtime_dir/weston.log" &
    weston_pid=$!

    for ((attempt = 0; attempt < 100; attempt++)); do
      [[ -S "$runtime_dir/$socket_name" ]] && break
      if ! kill -0 "$weston_pid" 2>/dev/null; then
        echo "Weston exited before creating its Wayland socket:" >&2
        tail -30 "$runtime_dir/weston.log" >&2 || true
        exit 1
      fi
      sleep 0.1
    done
    if [[ ! -S "$runtime_dir/$socket_name" ]]; then
      echo "Weston did not create its Wayland socket within 10 seconds." >&2
      tail -30 "$runtime_dir/weston.log" >&2 || true
      exit 1
    fi

    mkdir -p "$runtime_dir/home" "$runtime_dir/config" "$runtime_dir/data" "$runtime_dir/cache"
    : > "$runtime_dir/app.log"
    HOME="$runtime_dir/home" XDG_CONFIG_HOME="$runtime_dir/config" \
      XDG_DATA_HOME="$runtime_dir/data" XDG_CACHE_HOME="$runtime_dir/cache" \
      XDG_RUNTIME_DIR="$runtime_dir" WAYLAND_DISPLAY="$socket_name" \
      GDK_BACKEND=wayland WEBKIT_DISABLE_DMABUF_RENDERER=1 SSHBROWSE_HEADLESS_PREVIEW=1 \
      WAILS_MCP_HOST=127.0.0.1 WAILS_MCP_PORT=0 WAILS_MCP_TOKEN="$mcp_token" \
      "$runtime_dir/sshbrowse" >"$runtime_dir/app.log" 2>&1 &
    app_pid=$!
    mcp_url=""
    for ((attempt = 0; attempt < 300; attempt++)); do
      while IFS= read -r line; do
        if [[ "$line" =~ url=(http://127\.0\.0\.1:[0-9]+/mcp) ]]; then
          mcp_url="${BASH_REMATCH[1]}"
          break
        fi
      done < "$runtime_dir/app.log"
      [[ -n "$mcp_url" ]] && break
      if ! kill -0 "$app_pid" 2>/dev/null; then
        echo "Preview app exited before opening its MCP endpoint:" >&2
        tail -30 "$runtime_dir/app.log" >&2
        exit 1
      fi
      sleep 0.1
    done
    if [[ -z "$mcp_url" ]]; then
      echo "Preview app did not open its MCP endpoint within 30 seconds:" >&2
      tail -30 "$runtime_dir/app.log" >&2
      exit 1
    fi
    printf '%s\n' "$mcp_url" > "$runtime_dir/mcp-url"
    echo "Preview directory: $runtime_dir"
    echo "App MCP endpoint: $mcp_url"
    echo "Bearer token file: $runtime_dir/mcp-token"
    echo "Capture from another shell: $0 shot $runtime_dir /tmp/sshbrowse-settings.png"
    echo "Press Ctrl-C to stop the app and preview session."
    wait "$app_pid"
    ;;
  shot)
    [[ $# -ge 2 && $# -le 3 ]] || usage
    require_command weston-screenshooter
    runtime_dir="$2"
    if [[ ! -S "$runtime_dir/$socket_name" ]]; then
      echo "No running preview. Start one with: $0 run" >&2
      exit 1
    fi
    output="${3:-$PWD/sshbrowse-preview.png}"
    if [[ "$output" != /* ]]; then
      output="$PWD/$output"
    fi
    capture_dir="$(mktemp -d)"
    trap 'rm -rf -- "$capture_dir"' EXIT
    (cd "$capture_dir" && XDG_RUNTIME_DIR="$runtime_dir" WAYLAND_DISPLAY="$socket_name" weston-screenshooter)
    screenshots=("$capture_dir"/*.png)
    if [[ ${#screenshots[@]} -ne 1 || ! -f "${screenshots[0]}" ]]; then
      echo "Expected one PNG from weston-screenshooter." >&2
      exit 1
    fi
    mv -- "${screenshots[0]}" "$output"
    echo "$output"
    ;;
  *) usage ;;
esac
