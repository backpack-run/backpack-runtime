# Updating Backpack Runtime

Backpack checks for updates only when you explicitly run `backpack update`. It does not perform
background checks, download releases silently, or install third-party software.

Check the stable channel without downloading anything:

```text
backpack update --check
```

Download, verify, and install the newest stable release:

```text
backpack update
```

Alpha, beta, and release-candidate builds are excluded by default. Opt in explicitly:

```text
backpack update --check --prerelease
backpack update --prerelease
```

An exact immutable GitHub release can be selected independently of its channel:

```text
backpack update --version v0.1.0-alpha.1
```

## Verification and replacement

The updater uses the official `backpack-run/backpack-runtime` GitHub Releases API and accepts only
HTTPS API, archive, checksum, and redirect URLs. It selects the exact archive for the current
supported OS and architecture, verifies its SHA-256 value from the release's `checksums.txt`, and
extracts only the expected root-level `backpack` executable. Checksums and archives share GitHub as
their trust domain; SHA-256 detects corruption but is not an independent publisher signature.

On Linux and macOS, the verified executable is staged beside the current executable. Backpack then
renames the current binary to a timestamped rollback backup and atomically activates the staged
binary. If activation fails, it restores the backup before returning an error. This requires write
permission in the installation directory.

Windows does not safely permit a running executable to replace itself across all supported setups.
Backpack therefore leaves the current executable untouched and writes the verified binary beside it
as `backpack.exe.update-<version>`. It prints both exact paths and requires the user to exit all
Backpack processes, preserve the old executable as a rollback backup, and perform the replacement.
The version-pinned PowerShell installer remains the recommended automated Windows replacement path.

The updater never installs an unverified archive and never removes the current executable before a
verified replacement is ready.
