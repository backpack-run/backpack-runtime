# Security model

Backpack treats model repositories and manifests as semi-trusted data, not executable instructions. A manifest may select a trusted engine/version and describe typed artifacts; it cannot supply shell commands, Python requirements, worker code, indexes, or environment variables. Those decisions remain in the reviewed runtime and dependency catalogs.

## Implemented controls

- Model, runtime, and Python artifacts are pinned to immutable revisions or trusted URLs and SHA-256 digests.
- Split models must contain a complete contiguous shard set. Projectors and other auxiliary files are typed and verified.
- Archive extraction rejects traversal, absolute paths, links, devices, excessive entries, and unsupported formats, then atomically commits the verified directory.
- Python versions, dependency versions, indexes, and artifact hashes are catalog-owned. Environment reuse requires schema, lock, source, platform, worker, asset, and resolved-distribution metadata to match.
- SSH uses strict `known_hosts`, never stores private-key contents, binds remote inference to loopback, verifies remote content, and atomically finalizes transfers.
- The HTTP service refuses non-loopback binding while authentication is absent. Image content accepts inline `data:image/...` only; network and filesystem URLs are rejected.
- Uploaded audio and worker IPC use Backpack-controlled private paths. Generated artifact lookup is confined to the owning job directory.
- Diagnostics omit credentials, SSH connection details, prompts, conversations, and the user's home path prefix.
- Agent launchers use literal argv without a shell, child-only placeholder credentials, and Backpack-owned isolated config directories. Managed provider/model arguments cannot be replaced through passthrough flags. Backpack does not disable an agent's sandbox, approvals, or permission UI.

## Residual risk and release gates

Model parsers, native runtimes, Python wheels, and GPU drivers remain complex attack surfaces. Catalog updates require review of publisher identity, immutable source, digest, license, extraction shape, and runtime arguments. Z-Image and Wan must not deserialize arbitrary repository code or mutable pickle inputs; their package contracts remain blocked until this is demonstrated and executed on suitable hardware.

The current service is single-user and loopback-only. Authentication, multi-user isolation, and public network exposure are explicitly out of scope. Report vulnerabilities privately through GitHub Security Advisories as described in the root `SECURITY.md`.

Third-party agent executables remain independent trusted programs with their own update, telemetry, plugin, and workspace threat models. Backpack does not install them automatically. Initial isolated profiles disable nonessential hosted features, but users must still review the agent's own permissions before allowing file or shell changes.
