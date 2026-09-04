# Architecture

```text
CLI ---------\
Desktop ------> HTTP/API + Go services
clients ------/          |
                 resolved Model
                       x RuntimeAdapter
                       x ComputeTarget
                              |
                           Session
```

The model resolver maps a stable alias to a versioned catalog entry and immutable repository revision. The model manager consumes the package manifest, downloads to a temporary sibling, verifies SHA-256, atomically installs, and records installed state. It emits events and never prints UI text.

An adapter implements engine-specific preparation, launch, health, capability, and stop behavior. Selection uses `runtime.engine` from the normalized package contract. A compute target inspects and prepares a machine and executes a command. Therefore llama.cpp logic is shared by local and future SSH targets rather than becoming `LocalLlamaCpp` and `RemoteLlamaCpp` implementations.

Sessions bind one resolved model, adapter, and target to endpoint/process state. The current CLI manages foreground sessions; durable daemon-owned sessions are the next lifecycle increment.

Runtime state defaults to `%LOCALAPPDATA%/Backpack` on Windows and `~/.backpack` elsewhere, with separate models, manifests, runtimes, cache, logs, state, and config directories. `BACKPACK_HOME` provides an explicit test/development override.

The management API lives under `/api/backpack/v1`. OpenAI-compatible inference surfaces use `/v1` only where semantics match. The server is loopback-only until authentication and authorization exist.

