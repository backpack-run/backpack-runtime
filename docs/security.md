# Security model

Model repositories and manifests are semi-trusted data. They may select a reviewed engine/version and describe immutable artifacts; they cannot inject shell commands, runtime arguments, dependencies, indexes, environment variables, or executable worker code.

Implemented controls include immutable revisions and SHA-256 verification, complete split-set validation, safe archive extraction, atomic installs, strict SSH known-host handling, remote checksum verification, loopback-only inference services, private per-daemon launch credentials, diagnostics sanitization, literal argv invocation without a shell, and refusal of agent routing/model overrides through passthrough arguments.

Agent permissions remain owned by the agent. Backpack never adds approval-bypass flags. Cloud credentials remain in Backpack and are replaced—not forwarded—at the local proxy boundary. Desktop integration backups are private and restore refuses unexpected configuration drift.

Image, audio, media-job, and model-supplied Python execution paths are absent. Multimodal projector packages fail validation. Native model parsers, GPU drivers, external agent binaries, and llama.cpp remain independent attack surfaces and require reviewed updates.

The runtime is single-user and loopback-only. Public network exposure, multi-user authorization, and service tenancy are out of scope.
