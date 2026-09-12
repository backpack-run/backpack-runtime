# Install Backpack Runtime

Official release archives contain one `backpack` executable. Go, llama.cpp, whisper.cpp, uv, and Python do not need to be installed globally; Backpack installs trusted runtime bundles when a model first needs them.

## Installer behavior

The short commands below run the reviewed installer served by `backpack.run`. For a security-sensitive environment, use the inspect-first commands in each platform section instead: archive checksum verification protects the downloaded Backpack binary, while executing the installer itself still trusts the HTTPS endpoint.

The default channel is `latest`: the most recently published, non-draft GitHub Release, including a prerelease. The installer prints a warning when it selects a prerelease. Use `stable` to require the newest non-prerelease, or specify an exact `vVERSION` for a reproducible install. An exact command-line parameter takes precedence over `BACKPACK_VERSION`.

### Windows x64

The script supports Windows PowerShell 5.1 and PowerShell 7. It installs to `%LOCALAPPDATA%\Programs\Backpack\bin` unless `-InstallDirectory` is provided.

```powershell
irm https://backpack.run/install.ps1 | iex
```

Inspect first or pass an explicit channel/version:

```powershell
Invoke-WebRequest https://backpack.run/install.ps1 -OutFile install.ps1
Get-Content .\install.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 -Channel stable
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 -Version v0.1.0-alpha.1
$env:BACKPACK_VERSION = 'v0.1.0-alpha.1'; powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

### Linux amd64 and macOS arm64

The POSIX script installs to `~/.local/bin` unless a second positional argument is provided.

```sh
curl -fsSL https://backpack.run/install.sh | sh
```

Inspect first or select an explicit channel/version:

```sh
curl --fail --proto '=https' --proto-redir '=https' --tlsv1.2 \
  -o install.sh https://backpack.run/install.sh
less install.sh
sh install.sh
BACKPACK_CHANNEL=stable sh install.sh
sh install.sh v0.1.0-alpha.1
BACKPACK_VERSION=v0.1.0-alpha.1 sh install.sh
```

Linux and macOS runtime support may be narrower than archive availability. Check the compatibility matrix and run `backpack doctor` after installation.

## Security properties

Both installers:

- accept only an explicit OS/architecture allowlist;
- require HTTPS for initial requests and every redirect;
- reject drafts and validate release tags before constructing download URLs;
- verify the selected archive against its entry in the release `checksums.txt`;
- reject unexpected, duplicate, directory, and symbolic-link archive entries;
- extract only the executable into a private temporary directory;
- require the executable to report the exact selected version;
- stage replacement beside the destination, then replace it atomically;
- clean temporary and staging files on success or failure; and
- do not modify `PATH`, request elevation, or invoke a global package manager.

Open a new terminal or add the installation directory to `PATH` yourself. Re-running an installer safely replaces an existing installation when the executable is not currently locked by another process.

SHA-256 binds an archive to the checksum file, but both files share the GitHub Release trust boundary. Tagged releases additionally receive GitHub artifact attestations. See [Release provenance](release-provenance.md) for verification and trust assumptions.

The immutable `v0.1.0-alpha.1` PowerShell installer has a PowerShell 5.1 reinstall limitation. Remove its existing destination binary before reinstalling that exact version. The installer on later release tags uses a backup-assisted atomic replacement.

See [Uninstall](uninstall.md) to remove the executable or Backpack-managed data.

## `backpack.run` hosting contract

The public endpoints `https://backpack.run/install.sh` and `https://backpack.run/install.ps1` serve reviewed static copies of this repository's installer scripts. The deployment is sourced from [`amirthakatesh/backpack-web`](https://github.com/amirthakatesh/backpack-web) and uses a short cache lifetime so fixes can be rolled out without changing the URLs.

Static hosting must meet this contract:

- serve reviewed installer bytes from the release repository over HTTPS only;
- never redirect to HTTP or inject request-dependent script content;
- publish with `text/x-shellscript` or `text/plain` for shell and `text/plain` for PowerShell;
- give mutable channel URLs a short cache lifetime and tagged immutable copies a long `immutable` cache policy;
- deploy only after installer tests pass, retaining a traceable source commit/tag;
- keep binary and checksum downloads on immutable GitHub Release URLs unless an equivalently controlled, immutable mirror is introduced; and
- roll back by restoring a previously reviewed script, never by changing a release asset in place.

The website repository and deployment remain deliberately outside this runtime repository. A runtime source push does not publish a new installable binary: releases remain immutable and are selected through the GitHub Releases API.
