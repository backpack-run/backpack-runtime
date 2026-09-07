# v0.1.0-alpha.1 release-candidate qualification

Qualification date: 2026-09-07. Published tag source: `1baab8f4d2410a9dbe419732778f167a9f8ccbef`.

## Gate results

| Gate | Status | Evidence | Blocking? |
|---|---|---|---|
| Repository state | pass | `main` and `origin/main` matched at the candidate commit; worktree was clean before this report | no |
| Go format/test/vet/build | pass | Local `go test ./...`, `go vet ./...`, and `go build ./cmd/backpack`; cross-platform CI run [34148622725](https://github.com/backpack-run/backpack-runtime/actions/runs/34148622725) | no |
| Release configuration | pass | GoReleaser snapshot run [34148728536](https://github.com/backpack-run/backpack-runtime/actions/runs/34148728536) produced Windows amd64 ZIP, Linux amd64 tar.gz, macOS arm64 tar.gz, and SHA-256 checksums | no |
| Release archive contents | pass | Each archive contained the executable, `LICENSE`, `README.md`, and `THIRD_PARTY_NOTICES.md`; Unix executable mode was `0755` | no |
| Windows release-binary install | pass | SHA-256-verified snapshot ZIP was extracted into an isolated path containing spaces; manual uninstall and reinstall of the same archive passed | no |
| Windows CLI/diagnostics | pass | Snapshot `--version`, `doctor`, `doctor --json`, `models`, command help, and sanitized-path checks passed | no |
| Windows chat lifecycle | pass | Clean-home SmolLM2 135M pull, managed llama.cpp install, real prompt, detach, `ps`, stop, daemon crash/restart, and no-orphan checks passed | no |
| Windows Whisper | pass | Clean-home managed whisper.cpp/model installation transcribed the JFK fixture correctly | no |
| Windows Qwen3-ASR | pass | Clean-home managed Python 3.11/hash-locked environment transcribed the JFK fixture correctly | no |
| Windows Kokoro | pass | Clean-home managed Python 3.12/hash-locked environment produced a valid RIFF/WAVE file; overwrite refusal and `--force` replacement passed | no |
| Corruption recovery | pass | One-byte model corruption appeared in JSON doctor and was restored to the original digest; runtime corruption was detected and repaired; quarantined bundles are no longer enumerated as installed | no |
| Linux amd64 core GGUF | pass | Clean release archive, checksum verification, pull, managed llama.cpp install, real SmolLM2 CPU inference, detach, `ps`, stop, and orphan cleanup passed in run [34079088561](https://github.com/backpack-run/backpack-runtime/actions/runs/34079088561) | no |
| macOS arm64 | partial | Cross-build, checksum, archive membership, and executable mode passed; no real macOS host inference was available | no; experimental preview |
| Installer security | pass | Both scripts enforce HTTPS redirects, explicit version/platform selection, archive checksum verification, extraction allowlists, embedded-version checks, destination-directory staging, cleanup, and user-local installation without PATH edits/elevation | no |
| Public release | pass | Tag-triggered run [34149140443](https://github.com/backpack-run/backpack-runtime/actions/runs/34149140443) published a GitHub prerelease with all three platform archives and `checksums.txt` | no |
| Public Windows archive | pass | The immutable public ZIP matched SHA-256, contained only the expected four files, reported source `1baab8f`, and completed clean-home managed SmolLM2 inference | no |
| Public PowerShell installer | partial | Clean install and embedded-version validation passed. Same-version replacement failed under Windows PowerShell 5.1 because alpha.1 supplied a null `File.Replace` backup path; fixed on `main` | no; documented alpha limitation |
| Runtime/model notices | pass | Release archive carries notices; llama.cpp and uv license texts are separately pinned and installed; the Whisper worker bundle carries its component licenses and notice | no |

## Findings

### Release blocker

None remain in the qualified core.

Two blockers found during rehearsal were fixed: upstream Linux llama.cpp uses confined symlink chains, and repaired runtime quarantine directories were being reported as installed/corrupt. Regression tests cover both cases.

### Should fix in the next prerelease

- Ship the tested PowerShell 5.1 replacement fix in alpha.2. The immutable alpha.1 instructions document removing the existing binary before reinstalling.

### Safe to defer

- Independent third-party review and cryptographic signing/provenance can follow the first alpha. The installers deliberately avoid pipe-to-shell guidance and document that the checksum file shares the GitHub release trust domain with the archives.
- Package-manager distribution and self-update remain later work.

### Experimental / known limitation

- Linux amd64 remains a preview even though its CPU GGUF path passed qualification; audio variants are not published there.
- macOS arm64 is cross-built but has no real execution qualification.
- SSH has deterministic tests but no maintained real-host qualification.
- Whisper Large v3 Turbo CPU transcription is correct but slow on consumer hardware.
- Split GGUF, projector/vision, jobs, image, and video remain experimental or package-change-required as documented in the compatibility matrix.
