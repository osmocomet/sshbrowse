# Contributing to SSHBrowse

Bug reports, suggestions, and focused pull requests are welcome. Read [README.md](README.md) for product scope and use [SECURITY.md](SECURITY.md) for private vulnerability reports. Please redact credentials and private host details.

## Read this first

Focused bug fixes, reliability fixes, performance improvements, and maintenance work are welcome. Report bugs in issues so they can be tracked.

Discuss larger features, rewrites, architectural changes, and broad cleanup in an issue before implementing them so we can agree on the problem and scope.

Submitting a pull request does not obligate maintainers to review or merge it. They may close or defer it, suggest a smaller change, or implement the idea independently. Draft pull requests are also triaged and may be closed. Maintainers decide which contributions to accept.

Keep PRs focused, preserve unrelated work, and explain the problem, the change, and how you tested it. Include any platform limitations or validation gaps.

## Contribution terms

You retain ownership of your original contributions. By intentionally submitting a contribution to SSHBrowse, you confirm that you have the right to submit it and grant the SSHBrowse copyright holder a non-exclusive, perpetual, worldwide, irrevocable, royalty-free license to use, reproduce, modify, distribute, sublicense, and relicense it. This includes free versions for personal, non-commercial use and paid versions for professional, workplace, or commercial use.

Submitting a contribution does not grant you independent redistribution rights over SSHBrowse; [NOTICE](NOTICE) governs use of the project. Disclose any third-party material and its applicable licenses, and preserve required attribution and notices.

## Set up

Install Git, the Go version in `go.mod`, Node.js 24 with npm, and the Wails CLI version matching `go.mod`. Native builds need Xcode Command Line Tools on macOS; GCC, `pkg-config`, and GTK4/WebKitGTK 6.0 development packages on Linux; or WebView2 Runtime and Windows OpenSSH Client on Windows. Windows release builds also need Git Bash on `PATH` and NSIS 3.11 (`makensis`). Run `wails3 doctor` to check the toolchain.

```sh
git clone https://github.com/osmocomet/sshbrowse.git
cd sshbrowse
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
wails3 task dev
```

Put `$(go env GOPATH)/bin` on `PATH`. The first build installs frontend dependencies from the committed lockfile and fetches Go modules.

| Command | Purpose |
|---|---|
| `wails3 task dev` | Live development |
| `wails3 task check` | Go and frontend checks |
| `wails3 task verify` | Checks and native build; pass `ARCH=arm64` on macOS ARM64 |
| `wails3 task run` | Run the built app |
| `wails3 task package` | Build native packages; requires platform packaging tools |
| `wails3 task release VERSION=x.y.z` | Build release artifacts for the current native platform; requires platform packaging tools |

On Windows x64, run `wails3 task windows:release ARCH=amd64 VERSION=x.y.z` to create the installer and updater ZIP locally. This does not publish a release.

SSHBrowse supports Windows 11 x64 only. Setup checks for Microsoft Edge WebView2 Runtime, which is [normally included with Windows 11](https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution). If it is missing, Setup asks the user to install or repair it and stops before installing SSHBrowse. SSHBrowse does not download, bundle, or install the runtime. NSIS 3.11 remains pinned for reproducible packaging and to match the audited installer notices.

When updating dependencies, check the licenses of code included in the executable and frontend bundle, including copied code and generated helpers. Keep [third-party notices](docs/legal/THIRD_PARTY_NOTICES.txt) complete and preserve their license texts.

The optional Unix authentication regression needs `sshd`, `ssh-agent`, `ssh-add`, and `ssh-keygen`. It starts private loopback SSH servers and an agent with temporary keys; it does not use your SSH configuration or keys.

```sh
SSHBROWSE_SSH_INTEGRATION=1 go test ./internal/sshcmd -run '^TestSavedJumpAgentAuthentication$' -v
```

Test GUI and installer changes on the affected native platform. A Linux build does not establish macOS or Windows runtime behavior.

## Architecture

The frontend uses Svelte, TypeScript, and xterm.js. `main.go` and `internal/app` contain the Wails integration. Other packages stay independent of Wails: `internal/session` owns processes and terminals; `internal/sshcmd` builds OpenSSH arguments; `internal/profile` stores connections; `internal/sshconfig` discovers importable aliases; and `internal/buildinfo` supplies version details.

For headless Linux UI work, install Weston, then run `bash build/headless-preview.sh run` to start a private preview. In another shell, `bash build/headless-preview.sh shot <preview-dir> /tmp/sshbrowse.png` saves a screenshot. Keep the printed MCP token private and use separate worktrees for concurrent runs. Fedora 44 GNOME/Wayland remains the Linux acceptance target.
