# Architecture

```text
Codex / Claude Code / OpenCode / Pi / compatible client
                         |
        Responses / Messages / Chat Completions
                         |
              protocol translation layer
                         |
        model capability + inference/session layer
                         |
       Model x RuntimeAdapter x ComputeTarget
                         |
             local / SSH / compatible cloud
```

Backpack is neither the agent nor the inference engine. Agent behavior, tool approval, and workspace access remain with the external client. llama.cpp or a future trusted server backend executes model kernels. Backpack owns the boundary between them: model qualification, protocol translation, runtime installation, process/session ownership, context limits, and compute routing.

## Boundaries

Protocol handlers translate OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages into the protocol-neutral inference types in `internal/inference`. Runtime adapters never contain Codex-, Claude-, OpenCode-, or Pi-specific wire types. Unsupported protocol semantics fail explicitly instead of being approximated.

The trusted catalog separates factual capabilities from app compatibility. A model needs explicit coding and tool-calling capabilities, support for the app's protocol, and an app-specific `qualified` or `compatible-experimental` record before `backpack run <app>` admits it. Runtime execution status is not automatically an app qualification claim.

The model manager resolves immutable repositories, validates split GGUF sets and generic auxiliary data, resumes downloads only after a valid range response, verifies size and SHA-256, and atomically installs packages. Multimodal projectors are rejected because vision is outside the current coding-text scope.

The runtime manager maps the manifest's normalized `RuntimeRequirement` to a trusted platform/backend bundle. It verifies downloads, safely extracts archives, records installed file hashes, and permits multiple versions. llama.cpp is the sole shipped runtime family in this release.

Sessions bind a resolved model, runtime adapter, compute target, endpoint, and owned process. The daemon survives individual CLI calls, reuses compatible sessions, detects crashes, and stops children cleanly. On Windows it uses Job Objects; SSH execution preserves remote cache and loopback tunnel boundaries.

Authentication is target-scoped. Local execution needs no account, SSH uses the operator's credentials and known-host policy, and optional Backpack Cloud uses its own credential. The public runtime server remains loopback-only.

State defaults to `%LOCALAPPDATA%\Backpack` on Windows and `~/.backpack` elsewhere. `BACKPACK_HOME` is an explicit override for isolated test/development use. See [runtime lifecycle](runtime-lifecycle.md), [compute targets](compute-targets.md), and [state migrations](state-migrations.md).
