# Security

Report vulnerabilities privately through GitHub Security Advisories for `backpack-run/backpack-runtime`; do not open a public issue with exploit details. The API and internal workers are loopback-only. Do not log credentials, SSH keys, tokens, request secrets, or uploaded audio. Verify package/runtime checksums and immutable revisions before execution.

Only the trusted runtime and Python-environment catalogs may select executable code or dependency inputs. Model manifests select a declared engine/version but cannot inject shell commands or worker scripts. Managed Python distributions, package caches, environments, and configuration are contained under `BACKPACK_HOME`; Backpack does not inspect or mutate global Python. Uploaded transcription files use private temporary storage and are removed after each request.

SSH retains strict `known_hosts` checking, binds remote services to loopback, verifies content-addressed input/model/runtime transfers remotely, and does not store private key contents. Treat model formats that can execute code or deserialize pickle data as higher risk; only catalog-pinned trusted publishers should be eligible for those adapters.

Chat image inputs are restricted to inline `data:image/...` content; network and filesystem URLs are rejected. Generated artifacts are confined to Backpack-owned per-job directories, and API responses omit their internal absolute paths. `backpack doctor --json` is designed for bug reports and excludes prompts, conversations, credentials, SSH endpoint details, and the user's home path prefix.

Release installers require an explicit version, retain HTTPS across redirects, verify the archive against the release SHA-256 list, reject unexpected archive members and links, and stage user-local replacement without changing `PATH` or elevating privileges. Because the archive and checksum list share the GitHub Release trust domain, SHA-256 is corruption/tampering detection rather than an independent publisher signature. Protecting repository and workflow credentials remains essential.
