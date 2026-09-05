# Release readiness

This is a living gate checklist. A checked item has direct evidence; compiling a platform is not equivalent to executing on it.

## First public alpha

- [x] Windows clean-home model/runtime install and cache reuse
- [x] SmolLM2 135M chat
- [x] Qwen2.5 0.5B chat
- [x] SmolLM2 1.7B chat
- [x] Whisper Large v3 Turbo transcription
- [x] Qwen3-ASR 0.6B transcription from a hash-locked environment
- [x] Kokoro 82M speech from a hash-locked environment
- [x] checksum corruption detection and model repair
- [x] daemon/session stop and stale-state coverage
- [x] sanitized `backpack doctor` and JSON output
- [x] Windows amd64, Linux amd64, and macOS arm64 release archive configuration with SHA-256 checksums
- [x] checksum-verifying installer scripts and deterministic tests
- [x] CLI help smoke and Go format/test/vet/build gates
- [ ] clean Linux amd64 install and real core inference
- [ ] clean macOS arm64 install and real core inference
- [ ] independent installer/security review
- [ ] license/notice audit of every redistributed runtime archive
- [ ] publish a prerelease tag and verify its downloaded artifacts

Image, video, vision, larger GGUF models, and SSH may remain experimental and do not block alpha.

## Beta

- [ ] real SSH host doctor, resumable transfer, tunnel, inference, disconnect, and cleanup validation
- [ ] trusted/validated Linux and macOS audio runtime variants
- [ ] broader GGUF execution including complete split packages
- [ ] real projector/vision inference
- [ ] upgrade testing across at least two prerelease versions
- [ ] stable API compatibility and migration policy
- [ ] signed or provenance-attested release artifacts

## Stable

- [ ] repeatable Windows, Linux, and macOS release qualification
- [ ] documented support and security-response policy
- [ ] backward-compatible persisted-state migrations
- [ ] recovery testing for interrupted downloads, low disk, crashes, and OS sleep/resume
- [ ] sustained release/upgrade telemetry through opt-in reports or reproducible user diagnostics
- [ ] remove all unsupported claims from public surfaces and resolve critical audit findings

## Experimental media

Z-Image and Wan use hybrid, revision-pinned component package proposals today. They require a trusted dependency lock, deterministic component verification, fit/refusal behavior, cancellation, valid artifact metadata, cache reuse, cleanup, and real GPU inference before becoming supported.
