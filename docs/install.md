# Install Backpack Runtime

Official releases are designed to contain one `backpack` executable; Go, llama.cpp, whisper.cpp, uv, and Python do not need to be installed globally. Runtime engines are fetched later as trusted, versioned, SHA-256-verified bundles.

The first public alpha is available from [GitHub Releases](https://github.com/backpack-run/backpack-runtime/releases/tag/v0.1.0-alpha.1). Download the archive and `checksums.txt` from the matching release, verify SHA-256, then place `backpack` on `PATH`.

Prepared installers require an explicit semantic version and verify the release checksum before installation:

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/backpack-run/backpack-runtime/v0.1.0-alpha.1/scripts/install.ps1 -OutFile install.ps1
Get-Content .\install.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 -Version v0.1.0-alpha.1
```

```sh
curl --fail --proto '=https' --proto-redir '=https' --tlsv1.2 -o install.sh https://raw.githubusercontent.com/backpack-run/backpack-runtime/v0.1.0-alpha.1/scripts/install.sh
sh install.sh v0.1.0-alpha.1
```

Review the downloaded script before execution.

The scripts default to a user-local binary directory and use HTTPS for the initial request and every redirect. They reject unexpected, duplicate, directory, and link archive entries; extract only the expected executable; verify its embedded version; stage replacement in the destination directory; and clean temporary files on success or failure. They do not modify `PATH`, request elevation, or use global package managers. Open a new terminal or add `%LOCALAPPDATA%\Programs\Backpack\bin` on Windows or `~/.local/bin` on Linux/macOS to `PATH` yourself.

SHA-256 detects corrupt downloads and binds the archive to `checksums.txt`, but both are delivered by the same GitHub Release. It is not an independent signature: this flow assumes the repository, release workflow, GitHub account controls, and TLS delivery remain trusted. Release provenance/signing remains a beta hardening item.

The PowerShell script is intentionally documented as a downloaded script, not a pipe-to-shell command. Windows amd64, Linux amd64, and macOS arm64 archives are published; Linux/macOS are experimental previews for alpha.1.

The immutable alpha.1 PowerShell installer succeeds on a clean destination but cannot replace an existing binary under Windows PowerShell 5.1. Remove the existing destination binary before reinstalling alpha.1. The replacement implementation is fixed on `main` for the next prerelease.

Release channels follow semantic prerelease identifiers: `alpha` for early public compatibility, `beta` after broader platform/SSH validation, and an unqualified stable version only after the release-readiness gates are consistently met.
