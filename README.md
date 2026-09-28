# Backpack Runtime

**Backpack is an open-source runtime for running coding agents on open models—locally, on your own GPU, over SSH, or through compatible managed compute.**

Backpack is not a coding agent and it is not an inference engine. Codex, Claude Code, OpenCode, Pi, and compatible workspaces remain the agent layer; Backpack supplies a verified coding model, protocol translation, lifecycle management, and compute routing beneath them. llama.cpp performs local GGUF inference.

[![Release](https://img.shields.io/github/v/release/backpack-run/backpack-runtime?include_prereleases)](https://github.com/backpack-run/backpack-runtime/releases)
[![CI](https://github.com/backpack-run/backpack-runtime/actions/workflows/ci.yml/badge.svg)](https://github.com/backpack-run/backpack-runtime/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

This project is alpha software. Interfaces and the curated model set may change as agent compatibility is qualified.

## Install

Windows x64:

```powershell
irm https://backpack.run/install.ps1 | iex
```

Linux x64 and macOS arm64 preview:

```sh
curl -fsSL https://backpack.run/install.sh | sh
```

Installers select a published release, verify its SHA-256 checksum, and use a user-local directory. See [installation details](docs/install.md), [updates](docs/update.md), and [uninstall](docs/uninstall.md).

## Run a coding model

```console
backpack doctor
backpack models
backpack pull qwen3-coder-30b-a3b
backpack run qwen3-coder-30b-a3b --prompt "Write a Go function that deduplicates strings."
```

Qwen3-Coder 30B A3B needs substantial memory. Backpack performs a model-fit check before downloading or starting it and recommends remote compute when the current machine is unsuitable. `smollm2-135m` remains available through `backpack models --all` only as a fast runtime/CI fixture; it is not presented as an agent model.

## Launch a coding agent

Install the agent separately, then let Backpack configure a scoped connection to an eligible model:

```console
backpack launch list
backpack launch doctor codex --model qwen3-coder-30b-a3b
backpack launch codex --model qwen3-coder-30b-a3b
backpack launch claude --model qwen3-coder-30b-a3b
backpack launch opencode --model qwen3-coder-30b-a3b
backpack launch pi --model qwen3-coder-30b-a3b
```

Codex App and Claude App have persistent, restorable integrations on supported desktop platforms:

```console
backpack launch codex-app --model qwen3-coder-30b-a3b
backpack launch claude-app --model qwen3-coder-30b-a3b
backpack launch codex-app --restore
backpack launch claude-app --restore
```

Backpack refuses agent launch when the selected model lacks trusted coding, tool-calling, protocol, or agent-compatibility metadata. Model execution support and agent qualification are deliberately separate claims. See [agent launch](docs/launch.md) and [model qualification](docs/agent-model-qualification.md).

Backpack Cloud is optional and currently access-controlled. Local models, SSH, and the local API do not require an account:

```console
backpack login
backpack cloud models
backpack launch codex --model qwen3-coder-30b-a3b-instruct:cloud
```

## Supported agents and models

| Agent | Protocol | Integration |
| --- | --- | --- |
| Codex CLI / Codex App | OpenAI Responses | experimental, exercised by deterministic adapter tests |
| Claude Code / Claude App | Anthropic Messages | experimental, exercised by deterministic adapter tests |
| OpenCode | OpenAI-compatible Chat Completions | experimental |
| Pi | OpenAI-compatible Chat Completions | experimental |

The public catalog is intentionally curated. Qwen3-Coder 30B A3B is the current execution-qualified coding model and has experimental agent compatibility. Qwen3-Coder Next remains package/contract experimental until tool use is execution-qualified. See the [compatibility matrix](docs/model-compatibility.md) for precise claims.

## Execution and remote compute

```text
Coding Agent
     |
OpenAI / Anthropic protocol adapter
     |
Backpack inference + session layer
     |
Model x RuntimeAdapter x ComputeTarget
     |
llama.cpp on local / SSH / compatible cloud compute
```

Backpack verifies immutable model and runtime artifacts, selects CPU/CUDA/Vulkan/Metal variants, owns model sessions, reuses caches, and cleans up child processes. Advanced users can inspect them with `backpack ps`, `backpack stop`, and `backpack runtime list`.

SSH compute is experimental:

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute doctor gpu-1
backpack run qwen3-coder-30b-a3b --compute gpu-1
```

## API compatibility

`backpack serve` exposes a loopback-only API on `127.0.0.1:11434`:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages`
- Backpack session, model, compute, health, and event endpoints under `/api/backpack/v1`

The compatibility layers focus on coding-agent needs: streaming, roles, tools/tool results, usage, stop reasons, errors, cancellation, and context limits. Unsupported semantics return explicit errors. Backpack does not claim complete OpenAI or Anthropic API parity. See [API](docs/api.md).

## Development

Development requires Go 1.24 or newer:

```console
go test ./...
go vet ./...
go build ./cmd/backpack
```

Architecture details are in [docs/architecture.md](docs/architecture.md). Source code is Apache-2.0; models and managed engines retain their own licenses. Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
