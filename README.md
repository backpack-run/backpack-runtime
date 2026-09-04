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

- **Implemented:** versioned official catalog, schema-v1 manifest parsing, normalized runtime handshake, atomic downloads and SHA-256 verification, installed-model listing/inspection, local hardware report, llama.cpp local launch/health/chat/stop, loopback management API skeleton.
- **Experimental:** all GGUF models beyond SmolLM2 135M; foreground llama.cpp serving.
- **Planned:** persistent session manager, runtime installation, SSH execution, ASR/TTS workers, image/video jobs, Desktop client migration, managed Backpack Compute.

The first end-to-end proving model is intentionally `smollm2-135m`; larger GGUF packages are compatibility validation after the execution path works. See [model compatibility](docs/model-compatibility.md).

## Quick start

Requires Go 1.24+ and a `llama-server` binary on `PATH`, at `$BACKPACK_HOME/runtimes/llama.cpp/`, or identified by `BACKPACK_LLAMA_SERVER`.

```console
go build -o backpack ./cmd/backpack
backpack models
backpack pull smollm2-135m
backpack run smollm2-135m --prompt "Say hello"
backpack serve
```

The API binds to `127.0.0.1:11434` by default:

```console
curl http://127.0.0.1:11434/api/backpack/v1/health
curl http://127.0.0.1:11434/v1/models
```

The current runtime refuses non-loopback binds because remote API authentication is not implemented.

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

