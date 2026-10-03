#!/usr/bin/env bash

set -euo pipefail

repository_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
version="${VERSION:-0.1.0}"

case "$version" in
  ''|*[!0-9.]*|.*|*.)
    echo "VERSION must be a stable semantic version such as 0.1.0" >&2
    exit 1
    ;;
esac

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "VERSION must be a stable semantic version such as 0.1.0" >&2
  exit 1
fi

generated_dir="$(mktemp -d "${TMPDIR:-/tmp}/sshbrowse-wails-assets.XXXXXX")"
trap 'rm -rf -- "$generated_dir"' EXIT

cd "$repository_dir"
MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-15.0}" wails3 update build-assets \
  -name "sshbrowse" \
  -binaryname "sshbrowse" \
  -productname "SSHBrowse" \
  -productcompany "SSHBrowse" \
  -productidentifier "local.sshbrowse" \
  -productdescription "SSH session manager" \
  -productcopyright "Copyright © 2026. All rights reserved." \
  -productcomments "" \
  -productversion "$version" \
  -config "$repository_dir/build/config.yml" \
  -dir "$generated_dir" \
  -silent

# Wails emits a three-part fixed version and a neutral-only string table.
# Windows' version APIs read the four-part fixed fields and the en-US strings.
SSHBROWSE_RELEASE_VERSION="$version" node --input-type=module -e '
  import { readFileSync } from "node:fs";
  const info = JSON.parse(readFileSync(0, "utf8"));
  const version = process.env.SSHBROWSE_RELEASE_VERSION;
  info.fixed ??= {};
  info.fixed.file_version = `${version}.0`;
  info.fixed.product_version = `${version}.0`;
  const strings = info.info?.["0409"] ?? info.info?.["0000"] ?? {};
  strings.FileVersion = version;
  strings.ProductVersion = version;
  info.info = { "0409": strings };
  process.stdout.write(JSON.stringify(info, null, 2) + "\n");
' < "$generated_dir/windows/info.json" > "$generated_dir/windows/info.json.tmp"
mv "$generated_dir/windows/info.json.tmp" "$generated_dir/windows/info.json"

perl -0pi -e 's/<string>12\.0\.0<\/string>/<string>15.0.0<\/string>/g' \
  "$generated_dir/darwin/Info.plist" "$generated_dir/darwin/Info.dev.plist"

mkdir -p build/windows/nsis
cp "$generated_dir/darwin/Info.plist" build/darwin/Info.plist
cp "$generated_dir/darwin/Info.dev.plist" build/darwin/Info.dev.plist
cp "$generated_dir/windows/info.json" build/windows/info.json
cp "$generated_dir/windows/wails.exe.manifest" build/windows/wails.exe.manifest

# Wails includes two unused legacy association helpers. Strip only the known
# generated block from the temporary file, failing if its normalized hash moves.
perl -pi -e 's/[ \t]+$//' "$generated_dir/windows/nsis/wails_tools.nsh"
node --input-type=module - "$generated_dir/windows/nsis/wails_tools.nsh" <<'NODE'
import { createHash } from "node:crypto";
import { readFileSync, renameSync, writeFileSync } from "node:fs";

const [nshPath] = process.argv.slice(2);
const startMarker = "# Copy of APP_ASSOCIATE and APP_UNASSOCIATE macros from here https://gist.github.com/nikku/281d0ef126dbc215dd58bfd5b3a5cd5b\n";
const endMarker = "!macro wails.associateFiles\n";
const source = readFileSync(nshPath, "utf8").replace(/\r\n/g, "\n");
if (source.split(startMarker).length !== 2 || (source.match(/^!macro wails\.associateFiles$/gm) ?? []).length !== 1) {
  throw new Error("Unexpected Wails NSIS association macro boundaries");
}

const start = source.indexOf(startMarker);
const end = source.indexOf(endMarker, start + startMarker.length);
if (end < 0) {
  throw new Error("Wails NSIS association macro block is missing its expected end marker");
}

const block = source.slice(start, end);
const normalizedBlock = block.split("\n").map((line) => line.replace(/[ \t]+$/g, "")).join("\n");
const macroNames = [...normalizedBlock.matchAll(/^!macro\s+([^\s]+)/gm)].map((match) => match[1]);
const macroEnds = normalizedBlock.match(/^!macroend$/gm) ?? [];
const blockHash = createHash("sha256").update(normalizedBlock, "utf8").digest("hex");
if (macroNames.join(",") !== "APP_ASSOCIATE,APP_UNASSOCIATE" || macroEnds.length !== 2 ||
    blockHash !== "823e8d1d507d754097a784d212a154b9f25265786a8fa7fe9006d1f9cfeb46d4") {
  throw new Error("Unexpected Wails NSIS association macro block; inspect the upstream template before release");
}

let cleaned = source.slice(0, start) + source.slice(end);

// SSHBrowse checks the runtime prerequisite itself; never generate an installer
// helper that can deploy it. Check the upstream block before removing it.
const runtimeStart = cleaned.indexOf("# Install webview2 by launching");
const runtimeEnd = cleaned.indexOf("!macro wails.associateFiles\n", runtimeStart);
if (runtimeStart < 0 || runtimeEnd < 0 ||
    createHash("sha256").update(cleaned.slice(runtimeStart, runtimeEnd), "utf8").digest("hex") !==
      "d11cdb8e3b57772d63b42c137b2b5f7bdfee1310401bb7a78e71a2cf83f94cd2") {
  throw new Error("Unexpected Wails runtime installer helper; inspect the upstream template before release");
}
cleaned = cleaned.slice(0, runtimeStart) + cleaned.slice(runtimeEnd);
if (/^!macro APP_(?:ASSOCIATE|UNASSOCIATE)\b/m.test(cleaned)) {
  throw new Error("Wails NSIS association macro definitions remain after postprocessing");
}

const temporaryPath = `${nshPath}.tmp`;
writeFileSync(temporaryPath,
  "# SSHBrowse removes the upstream runtime installer and legacy association helpers.\n" + cleaned,
  "utf8");
renameSync(temporaryPath, nshPath);
NODE
cp "$generated_dir/windows/nsis/wails_tools.nsh" build/windows/nsis/wails_tools.nsh

# Keep the generated manifest aligned with the supported Windows target. The
# Wails template still includes the Windows 7/8 fallback and a legacy fallback
# value that the project no longer supports.
perl -ni -e '
  next if /<dpiAware\b/;
  if (/<dpiAwareness\b/) {
    s/permonitorv2,permonitor/permonitorv2/;
    s/\s*<!--.*?-->//;
  }
  print;
' build/windows/wails.exe.manifest
