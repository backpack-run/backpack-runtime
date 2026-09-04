# Backpack Runtime

Backpack Runtime is the open execution layer for the Backpack open-model ecosystem. The `backpack` CLI, Backpack Desktop, and third-party clients are intended to use the same model installation, runtime selection, process lifecycle, compute targeting, and HTTP API.

It is **not** a new inference engine. It orchestrates engines such as llama.cpp, whisper.cpp, pinned Python workers, and GPU generation runtimes.

```text
CLI / Desktop / third-party clients
                 |
          Backpack Runtime
                 |
     Model x RuntimeAdapter x ComputeTarget
```

## Status

- **Implemented:** versioned model/runtime catalogs, manifest-driven selection, verified atomic model and llama.cpp runtime installs, auto-started local service, persisted sessions, `ps`/`stop`, hardware inspection, and llama.cpp launch/health/chat/SSE/stop.
- **Experimental:** managed-runtime SSH GGUF execution and all GGUF models beyond SmolLM2 135M.
- **Planned:** Whisper and isolated ASR/TTS workers, image/video jobs, Desktop migration, and managed Backpack Compute.

The first end-to-end proving model is intentionally `smollm2-135m`; larger GGUF packages are compatibility validation after the execution path works. See [model compatibility](docs/model-compatibility.md).

## Quick start

After installing a Backpack binary (the release pipeline is prepared; the first public release is still pending):

```console
backpack models
backpack pull smollm2-135m
backpack run smollm2-135m --prompt "Say hello"
backpack run smollm2-135m --detach
backpack ps
backpack stop <session-id>
backpack serve
```

Backpack selects a CPU, CUDA, Vulkan, or Metal llama.cpp bundle for the machine, verifies its SHA-256 digest, and installs it automatically. `backpack runtime list`, `show`, `install`, and `verify` provide explicit inspection and repair controls. Developers may still set `BACKPACK_LLAMA_SERVER` to test a local build.

The API binds to `127.0.0.1:11434` by default:

```console
curl http://127.0.0.1:11434/api/backpack/v1/health
curl http://127.0.0.1:11434/v1/models
curl -N http://127.0.0.1:11434/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"smollm2-135m","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

The current runtime refuses non-loopback binds because remote API authentication is not implemented.

## SSH compute (experimental)

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute test gpu-1
backpack run smollm2-135m --compute gpu-1
```

Backpack transfers its locally verified runtime bundle into the remote user-owned cache, verifies every file remotely, reuses runtime/model caches, and tunnels loopback inference over SSH. No global remote `llama-server` or root access is required.

## Repositories

- `backpack-model-packager` produces model artifacts, manifests, checksums, and runtime-service bundles.
- `backpack-runtime` consumes that contract and owns execution/model state.
- Backpack Desktop should become an API client while retaining UI and user workflow state.

## Development

Development requires Go 1.24+:

```console
go test ./...
go vet ./...
go build ./cmd/backpack
```

Key code lives under `internal/models`, `internal/runtime`, `internal/adapters`, `internal/compute`, `internal/server`, and `internal/cli`. Architecture and implementation details are in [docs](docs/architecture.md).

## Security and licensing

Read [SECURITY.md](SECURITY.md) before exposing or embedding the runtime. Source code is Apache-2.0. Runtime engines and models keep their own licenses; packaging never relicenses a model.
