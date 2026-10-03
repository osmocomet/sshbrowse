# SSHBrowse

SSHBrowse is a cross-platform desktop app for managing SSH, SFTP, and local terminal sessions. It is built for people looking for a modern alternative to SecureCRT, MobaXterm, and other session managers.

Save and organize connections, open sessions in tabs or tiles, and send commands or live input to multiple terminals at once. SSHBrowse runs on macOS, Windows, and Linux.

SSHBrowse is currently in beta. Feedback, bug reports, and contributions are welcome.

[Releases](https://github.com/osmocomet/sshbrowse/releases) · [Report a bug](https://github.com/osmocomet/sshbrowse/issues/new/choose) · [Contribute](CONTRIBUTING.md)

[![Four SSHBrowse sessions connected to an EVPN network lab in a tiled workspace](assets/network-lab.png)](assets/network-lab.png)

## Your OpenSSH, not ours

The actual SSH and SFTP connections, authentication, host-key verification, and SSH configuration are handled by your system's `ssh` and `sftp` clients.

SSHBrowse does not implement the SSH protocol itself, proxy SSH traffic through another service, or store your passwords or private keys.

## Highlights

- Save and organize SSH connections in folders
- Import aliases from your OpenSSH config
- Choose a saved connection as a jump host, or enter a raw ProxyJump route
- Open SSH, SFTP, and local terminal sessions
- Work with tabs or tiled terminals
- Send commands or live input to multiple sessions

## Platforms

SSHBrowse provides native packages for macOS, Windows, and Linux. The following platforms have been tested with packaged releases:

| Platform | Package | Notes |
| --- | --- | --- |
| macOS 15+ (Apple silicon and Intel) | Universal DMG | Native packaged acceptance performed on Apple silicon. Ad-hoc signed, not notarized; Gatekeeper may require manual approval. |
| Linux x86-64 | DEB or RPM | Fedora 44 GNOME/Wayland RPM and Ubuntu 26.04 GNOME/Wayland DEB accepted natively. Other distributions need GTK4 and WebKitGTK 6.0 and are less tested. Local packages are not repository-signed. |
| Windows 11 x64 only | Per-user installer | Native acceptance has covered the installer and core SSH/SFTP behavior. Requires the Windows OpenSSH Client and Microsoft Edge WebView2 Runtime, normally included with Windows 11; Setup stops if WebView2 is missing. SSHBrowse does not bundle, download, or install WebView2. The installer is unsigned; SmartScreen may warn. |

For release downloads, check the architecture and compare the SHA-256 hash with `SHA256SUMS`.

## Get started

Download the latest package from [Releases](https://github.com/osmocomet/sshbrowse/releases).

After launching SSHBrowse, you can:

- Create a saved connection
- Import aliases with **Shell > Import from SSH Config…**
- Enter `user@host:port` directly in the new-tab picker

## Help and contributing

Packaged releases check GitHub for updates at startup, at most once every 24 hours. Disable this in **Settings > General > Updates**. Downloads and restarts require your approval; Linux notifications link to the release so you can install the updated DEB or RPM package.

For connection problems, try the same destination with system OpenSSH (`ssh alias` or `ssh user@host`). Use the [bug report form](https://github.com/osmocomet/sshbrowse/issues/new/choose) for bugs and [SECURITY.md](SECURITY.md) for private vulnerability reports. Redact credentials and private host details.

Bug reports, suggestions, and focused pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, contribution terms, and guidance on discussing larger changes first.

## Licensing

SSHBrowse is free for personal, non-commercial use. Professional, workplace, or commercial use requires a paid license.

The source code is publicly available for inspection and private personal modification. Independent redistribution, including modified versions, requires authorization; GitHub's viewing and forking rights still apply.

See [LICENSE](LICENSE) for SSHBrowse's terms and [THIRD_PARTY_NOTICES.txt](docs/legal/THIRD_PARTY_NOTICES.txt) for third-party licenses. Run `sshbrowse --licenses` to print both.
