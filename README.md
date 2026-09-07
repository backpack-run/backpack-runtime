# Backpack Runtime

**Backpack Runtime is an open-source runtime and CLI for running Backpack-packaged open models locally or on remote compute, with a programmatic HTTP API for external clients.**

It is **not** a new inference engine. It orchestrates engines such as llama.cpp, whisper.cpp, pinned Python workers, and GPU generation runtimes.

```text
CLI / Go client / external clients
                 |
          Backpack Runtime
                 |
     Model x RuntimeAdapter x ComputeTarget
```

## Status

- **Release channel:** early alpha; APIs and behavior may change.
- **Windows x64:** supported and execution-validated for this alpha.
- **Linux x64 and macOS arm64:** experimental preview binaries. Linux GGUF qualification is tracked separately; macOS has not received real inference validation.
- **Validated models on Windows:** SmolLM2 135M/1.7B and Qwen2.5 0.5B chat through managed llama.cpp, Whisper Large v3 Turbo transcription, Qwen3-ASR transcription, and Kokoro speech.
- **Experimental:** split GGUF, projector/vision contracts, managed-runtime SSH execution, and the generic media-job API.
- **Package/runtime work required:** Z-Image and Wan. Their immutable component inventories are understood, but no execution-validated GPU adapter is shipped.

The first end-to-end proving model is intentionally `smollm2-135m`; larger GGUF packages are compatibility validation after the execution path works. See [model compatibility](docs/model-compatibility.md).

## Install and quick start

The first public release is still pending. Once `v0.1.0-alpha.1` exists, download the installer from that immutable tag, inspect it, then run it with the explicit version:

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/backpack-run/backpack-runtime/v0.1.0-alpha.1/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -Version v0.1.0-alpha.1
```

Linux x64 and macOS arm64 preview:

```sh
curl --fail --proto '=https' --proto-redir '=https' --tlsv1.2 -o install.sh https://raw.githubusercontent.com/backpack-run/backpack-runtime/v0.1.0-alpha.1/scripts/install.sh
sh install.sh v0.1.0-alpha.1
```

No pipe-to-shell installation is recommended. The installers verify the release archive against the matching GitHub Release checksum before replacing a user-local binary. Then try the smallest model first:

```console
backpack version
backpack doctor
backpack models
backpack pull smollm2-135m
backpack run smollm2-135m --prompt "Explain Backpack Runtime in one sentence."
backpack transcribe sample.wav --model whisper-large-v3-turbo
backpack speak "Hello from Backpack" --model kokoro-82m --output hello.wav
backpack run smollm2-135m --detach
backpack ps
backpack stop <session-id>
backpack serve
backpack doctor --json
```

Backpack selects a CPU, CUDA, Vulkan, or Metal llama.cpp bundle for the machine, verifies its SHA-256 digest, and installs it automatically. `backpack runtime list`, `show`, `install`, and `verify` provide explicit inspection and repair controls. Developers may still set `BACKPACK_LLAMA_SERVER` to test a local build.

The API binds to `127.0.0.1:11434` by default:

```console
curl http://127.0.0.1:11434/api/backpack/v1/health
curl http://127.0.0.1:11434/v1/models
curl -N http://127.0.0.1:11434/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"smollm2-135m","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

The current runtime refuses non-loopback binds because remote API authentication is not implemented.

The CLI does not require Go, llama.cpp, whisper.cpp, Python, qwen-asr, or Kokoro to be installed globally. Native engines, Python distributions, and isolated environments are installed into the Backpack data directory from pinned runtime definitions. Python runtimes are currently limited to Windows x64 CPU.

Backpack stores models, runtime bundles, state, logs, and generated outputs under `%LOCALAPPDATA%\Backpack` on Windows and `~/.backpack` on Linux/macOS. Set `BACKPACK_HOME` only when you intentionally need an isolated location. Use `backpack list`, `backpack runtime list`, and `backpack doctor` to inspect it; use `backpack ps` and `backpack stop <session-id>` to cleanly stop loaded models.

## SSH compute (experimental)

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute doctor gpu-1
backpack run smollm2-135m --compute gpu-1
```

Backpack transfers its locally verified runtime bundle into the remote user-owned cache, verifies every file remotely, reuses runtime/model caches, and tunnels loopback inference over SSH. `compute test` and `compute doctor` are aliases for the same real readiness probe. rsync resumes partial transfers when both ends provide it; SCP is the non-resumable fallback. No global remote `llama-server` or root access is required.

Download installation and checksum-verifying scripts are documented in [docs/install.md](docs/install.md). Do not use a shell-pipe installer URL until a reviewed release is published.

## Repositories

- `backpack-model-packager` produces model artifacts, manifests, checksums, and runtime-service bundles.
- `backpack-runtime` consumes that contract and owns execution/model state.
- External applications can consume the public HTTP API or `pkg/client` without taking ownership of runtime processes.

## Development

Development requires Go 1.24+:

```console
go test ./...
go vet ./...
go build ./cmd/backpack
```

Key code lives under `internal/models`, `internal/runtime`, `internal/adapters`, `internal/compute`, `internal/server`, and `internal/cli`. Architecture and implementation details are in [docs](docs/architecture.md).

## Security and licensing

Read [SECURITY.md](SECURITY.md) before exposing or embedding the runtime. Report ordinary alpha bugs through [GitHub Issues](https://github.com/backpack-run/backpack-runtime/issues) and vulnerabilities privately through GitHub Security Advisories. Source code is Apache-2.0. Runtime engines and models keep their own licenses; packaging never relicenses a model.
