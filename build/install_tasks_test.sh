#!/usr/bin/env bash

set -euo pipefail

repository_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_dir"

case "$(uname -s)" in
  Linux)
    platform=linux
    ;;
  Darwin)
    platform=darwin
    ;;
  *)
    echo "install task regression test requires Linux or macOS" >&2
    exit 77
    ;;
esac

stage="$(mktemp -d "${TMPDIR:-/tmp}/sshbrowse-install-tasks.XXXXXX")"
marker="$stage/command-substitution-ran"
config="$stage/config"
sentinels="$stage/sentinels"
mkdir -p "$config/sshbrowse" "$sentinels"
trap 'rm -rf -- "$stage"' EXIT

printf '%s\n' '{"sentinel":"connections"}' > "$config/sshbrowse/connections.json"
printf '%s\n' '{"sentinel":"window"}' > "$config/sshbrowse/window.json"
cp "$config/sshbrowse/connections.json" "$sentinels/connections.json"
cp "$config/sshbrowse/window.json" "$sentinels/window.json"

ordinary_destination="$stage/ordinary"
# Every metacharacter below is part of the directory name. In particular, the
# escaped command substitution must remain text and never create $marker.
unusual_destination="$stage/prefix space 'single' \"double\" dollar\$ backtick\` backslash\\ reserved#?\&\; literal\$(touch \"${marker}\")"
percent_destination="$stage/prefix percent%"

run_task() {
  local action="$1"
  local destination="$2"
  if [[ "$platform" == linux ]]; then
    env XDG_CONFIG_HOME="$config" wails3 task "$action" "INSTALL_ROOT=$destination"
  else
    env XDG_CONFIG_HOME="$config" wails3 task "$action" "INSTALL_DIR=$destination"
  fi
}

validate_linux_desktop_entry() {
  local desktop_file="$1"
  if command -v desktop-file-validate >/dev/null 2>&1; then
    desktop-file-validate "$desktop_file"
  fi
  python3 - "$desktop_file" <<'PY'
import sys

import gi

gi.require_version("Gio", "2.0")
from gi.repository import Gio

desktop_file = sys.argv[1]
app_info = Gio.DesktopAppInfo.new_from_filename(desktop_file)
if app_info is None:
    raise SystemExit(f"GIO could not load {desktop_file!r}")
if not app_info.get_commandline():
    raise SystemExit(f"GIO loaded {desktop_file!r} without an Exec command")
PY
}

launch_linux_desktop_entry() {
  local desktop_file="$1"
  local binary_file="$2"
  local backup_file="$binary_file.test-real.$$"
  local true_binary
  true_binary="$(type -P true)"
  if [[ -z "$true_binary" ]]; then
    echo "cannot find a harmless executable for the GIO launch check" >&2
    exit 1
  fi

  # Let GIO exercise the generated path without opening the real GUI.
  mv -- "$binary_file" "$backup_file"
  ln -s -- "$true_binary" "$binary_file"
  local launch_status=0
  if ! python3 - "$desktop_file" <<'PY'
import sys

import gi

gi.require_version("Gio", "2.0")
from gi.repository import Gio

app_info = Gio.DesktopAppInfo.new_from_filename(sys.argv[1])
if app_info is None:
    raise SystemExit(f"GIO could not load {sys.argv[1]!r}")
if not app_info.launch([], None):
    raise SystemExit("GIO refused to launch the generated desktop entry")
PY
  then
    launch_status=1
  fi
  mv -Tf -- "$backup_file" "$binary_file"
  return "$launch_status"
}

exercise_destination() {
  local destination="$1"
  if [[ "$platform" == linux ]]; then
    mkdir -p \
      "$destination/bin" \
      "$destination/share/applications" \
      "$destination/share/icons/hicolor/256x256/apps"
  fi
  run_task install "$destination"
  if [[ "$platform" == linux ]]; then
    local binary="$destination/bin/sshbrowse"
    local desktop="$destination/share/applications/io.github.osmocomet.sshbrowse.desktop"
    test -x "$binary"
    test -f "$desktop"
    validate_linux_desktop_entry "$desktop"
    launch_linux_desktop_entry "$desktop" "$binary"
  else
    test -x "$destination/sshbrowse.app/Contents/MacOS/sshbrowse"
  fi
  run_task update "$destination"
  if [[ "$platform" == linux ]]; then
    test -x "$destination/bin/sshbrowse"
  else
    test -x "$destination/sshbrowse.app/Contents/MacOS/sshbrowse"
  fi

  run_task uninstall "$destination"
  if [[ "$platform" == linux ]]; then
    test ! -e "$destination/bin/sshbrowse"
    test ! -e "$destination/share/applications/io.github.osmocomet.sshbrowse.desktop"
    test ! -e "$destination/share/icons/hicolor/256x256/apps/sshbrowse.png"
  else
    test ! -e "$destination/sshbrowse.app"
  fi
}

expect_linux_percent_rejection() {
  local action="$1"
  local output
  if output="$(run_task "$action" "$percent_destination" 2>&1)"; then
    echo "$action unexpectedly accepted a percent-containing Linux prefix" >&2
    exit 1
  fi
  case "$output" in
    *"INSTALL_ROOT cannot contain '%'"*) ;;
    *)
      echo "$action rejected the percent-containing prefix for an unexpected reason:" >&2
      echo "$output" >&2
      exit 1
      ;;
  esac
  cmp "$sentinels/percent-binary" "$percent_destination/bin/sshbrowse"
  cmp "$sentinels/percent-desktop" "$percent_destination/share/applications/io.github.osmocomet.sshbrowse.desktop"
  cmp "$sentinels/percent-icon" "$percent_destination/share/icons/hicolor/256x256/apps/sshbrowse.png"
}

exercise_destination "$ordinary_destination"
exercise_destination "$unusual_destination"

if [[ "$platform" == linux ]]; then
  mkdir -p \
    "$percent_destination/bin" \
    "$percent_destination/share/applications" \
    "$percent_destination/share/icons/hicolor/256x256/apps"
  printf '%s\n' 'pre-existing binary' > "$percent_destination/bin/sshbrowse"
  printf '%s\n' 'pre-existing desktop entry' > "$percent_destination/share/applications/io.github.osmocomet.sshbrowse.desktop"
  printf '%s\n' 'pre-existing icon' > "$percent_destination/share/icons/hicolor/256x256/apps/sshbrowse.png"
  chmod 0755 "$percent_destination/bin/sshbrowse"
  cp "$percent_destination/bin/sshbrowse" "$sentinels/percent-binary"
  cp "$percent_destination/share/applications/io.github.osmocomet.sshbrowse.desktop" "$sentinels/percent-desktop"
  cp "$percent_destination/share/icons/hicolor/256x256/apps/sshbrowse.png" "$sentinels/percent-icon"
  expect_linux_percent_rejection install
  expect_linux_percent_rejection update
fi

if [[ -e "$marker" ]]; then
  echo "destination command substitution executed" >&2
  exit 1
fi
cmp "$sentinels/connections.json" "$config/sshbrowse/connections.json"
cmp "$sentinels/window.json" "$config/sshbrowse/window.json"

echo "installation tasks preserved literal destinations and user data on $platform"
