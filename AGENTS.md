# SSHBrowse maintenance guide

Read [README.md](README.md) for product scope and [CONTRIBUTING.md](CONTRIBUTING.md) for setup, architecture, and contribution terms.

## Boundaries

- Let the platform OpenSSH client own authentication, host keys, SSH configuration, and the protocol. Do not reimplement them.
- Keep Wails imports in `main.go` and `internal/app`; other packages stay independent of Wails.
- Add complexity only for observed problems. Avoid speculative fallbacks, plugins, and settings.

## Changes

- Keep changes focused, explicit, and small. Preserve unrelated work and behavior outside the task.
- Handle errors deliberately; explain any intentionally ignored error. Bound waits, retries, buffers, and queues.
- Use descriptive names, comments for non-obvious constraints, and arithmetic byte sizes such as `32 * 1024`.
- Justify new dependencies. Test intricate logic and regressions; exercise integration and GUI behavior in the running app.

## Validation and Git

- Match checks to the change. Use `wails3 task verify` on the native host (`ARCH=arm64` for macOS ARM64). For build, packaging, or GUI changes, package and smoke-run the native app. Linux acceptance targets Fedora 44 x86-64 GNOME/Wayland and a runnable `bin/sshbrowse`.
- For headless Linux GUI work, use the preview and screenshot procedure in [CONTRIBUTING.md](CONTRIBUTING.md); inspect the screenshot. Give concurrent workers separate worktrees and preview runs, with private tokens and distinct ports.
- Inspect the final diff and report validation gaps. Before committing, obtain an independent adversarial review of the staged diff; use a separate review agent when working with coding agents.
- Use short imperative commit subjects. Keep documentation concise and free of relative dates. Do not push, publish, merge, or create remote resources unless asked.
