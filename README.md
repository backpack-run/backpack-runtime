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

- **Implemented:** versioned catalog, manifest-driven selection, verified atomic installs, auto-started local service, persisted sessions, `ps`/`stop`, hardware inspection, and llama.cpp launch/health/chat/SSE/stop.
- **Experimental:** SSH GGUF execution and all GGUF models beyond SmolLM2 135M.
- **Planned:** runtime bundle installation, ASR/TTS workers, image/video jobs, Desktop migration, and managed Backpack Compute.

The first end-to-end proving model is intentionally `smollm2-135m`; larger GGUF packages are compatibility validation after the execution path works. See [model compatibility](docs/model-compatibility.md).

## Quick start

Requires Go 1.24+ and a `llama-server` binary on `PATH`, at `$BACKPACK_HOME/runtimes/llama.cpp/`, or identified by `BACKPACK_LLAMA_SERVER`.

```console
go build -o backpack ./cmd/backpack
backpack models
backpack pull smollm2-135m
backpack run smollm2-135m --prompt "Say hello"
backpack run smollm2-135m --detach
backpack ps
backpack stop <session-id>
backpack serve
```

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

The remote host must already provide a compatible `llama-server`; Backpack verifies known hosts and model checksums, reuses the remote cache, and tunnels inference over SSH.

## Repositories

- `backpack-model-packager` produces model artifacts, manifests, checksums, and runtime-service bundles.
- `backpack-runtime` consumes that contract and owns execution/model state.
- Backpack Desktop should become an API client while retaining UI and user workflow state.

## Development

```console
go test ./...
go vet ./...
go build ./cmd/backpack
```

Key code lives under `internal/models`, `internal/runtime`, `internal/adapters`, `internal/compute`, `internal/server`, and `internal/cli`. Architecture and implementation details are in [docs](docs/architecture.md).

## Security and licensing

Read [SECURITY.md](SECURITY.md) before exposing or embedding the runtime. Source code is Apache-2.0. Runtime engines and models keep their own licenses; packaging never relicenses a model.
