# Install Backpack Runtime

Official releases are designed to contain one `backpack` executable; Go, llama.cpp, whisper.cpp, uv, and Python do not need to be installed globally. Runtime engines are fetched later as trusted, versioned, SHA-256-verified bundles.

No public alpha tag has been published yet. After a release is published, download the archive and `checksums.txt` from the matching GitHub release, verify SHA-256, then place `backpack` on `PATH`.

Prepared installers require an explicit semantic version and verify the release checksum before installation:

```powershell
.\scripts\install.ps1 -Version v0.1.0-alpha.1
```

```sh
./scripts/install.sh v0.1.0-alpha.1
```

The scripts default to a user-local binary directory and use HTTPS only. The PowerShell script is intentionally documented as a downloaded script, not a pipe-to-shell command, until it has independent review and a controlled hosting path. Windows amd64, Linux amd64, and macOS arm64 archives are configured; a tag is not created automatically.

Release channels follow semantic prerelease identifiers: `alpha` for early public compatibility, `beta` after broader platform/SSH validation, and an unqualified stable version only after the release-readiness gates are consistently met.
