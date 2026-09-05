# Architecture

```text
CLI ---------\
Go client -----> HTTP/API + Go services
other clients -/          |
                 resolved Model
                       x RuntimeAdapter
                       x ComputeTarget
                              |
                           Session
```

The model resolver maps a stable alias to a versioned catalog entry and immutable repository revision. The model manager consumes the package manifest, downloads to a temporary sibling, verifies SHA-256, atomically installs, and records installed state. It emits events and never prints UI text.

A normalized `RuntimeRequirement` is resolved by the runtime manager against a separate trusted catalog. The manager inspects the compute target, selects a platform/backend variant, downloads only HTTPS artifacts, verifies catalog-pinned SHA-256 digests, safely extracts them, writes a per-file installed manifest, and atomically commits a versioned runtime directory. Multiple versions coexist. Adapters receive an installed executable and do not own download policy.

An adapter implements engine-specific preparation, launch, health, capability, and stop behavior. Selection uses `runtime.engine` from the normalized package contract. A compute target inspects and prepares a machine and executes a command. Native Whisper uses a job-style adapter; qwen-asr and Kokoro use persistent isolated-Python workers. API handlers route by capability, never by engine or model ID.

Sessions bind one resolved model, adapter, and target to endpoint/process state. The auto-started local service owns processes beyond an individual CLI request, while CLI and the public Go client use the same HTTP contract. See [runtime lifecycle](runtime-lifecycle.md) and [SSH compute](ssh-compute.md).

Before launch, the model-fit policy compares manifest estimates and artifact size with target RAM/VRAM. Clearly unsafe local fallback is refused with a remote-compute recommendation unless the caller explicitly forces it. See [model fit](model-fit.md).

Runtime state defaults to `%LOCALAPPDATA%/Backpack` on Windows and `~/.backpack` elsewhere, with separate models, manifests, runtimes, cache, logs, state, and config directories. Managed runtimes use `runtimes/<engine>/<version>/<variant>`. `BACKPACK_HOME` provides an explicit test/development override.

The management API lives under `/api/backpack/v1`. OpenAI-compatible inference surfaces use `/v1` only where semantics match. The server is loopback-only until authentication and authorization exist.
