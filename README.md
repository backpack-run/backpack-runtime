# Backpack Runtime

**Backpack runs AI coding workspaces on curated open models—locally, on your own GPU, over SSH, or through compatible managed compute.**

Backpack is not a coding agent and it is not an inference engine. Codex, Claude Code, OpenCode, Pi, and compatible workspaces provide the coding experience. Backpack supplies the verified model, protocol translation, lifecycle management, and compute routing beneath them; llama.cpp performs local GGUF inference.

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

The installers select a published release, verify its SHA-256 checksum, and install to a user-local directory. See [installation](docs/install.md), [updates](docs/update.md), and [uninstall](docs/uninstall.md).

## Run a coding workspace

Install your preferred coding app separately, then choose an eligible model:

```console
backpack doctor
backpack models
backpack pull qwen3-coder-30b-a3b
backpack run codex --model qwen3-coder-30b-a3b
```

The same model can power another supported workspace:

```console
backpack run claude --model qwen3-coder-30b-a3b
backpack run opencode --model qwen3-coder-30b-a3b
backpack run pi --model qwen3-coder-30b-a3b
```

Use `backpack run list` to inspect installed apps and eligible-model counts, and `backpack run doctor <app> --model <model>` for actionable readiness checks. Arguments after `--` go directly to CLI apps without a shell.

Codex App and Claude App have persistent, explicitly restorable integrations on supported desktop platforms:

```console
backpack run codex-app --model qwen3-coder-30b-a3b
backpack run claude-app --model qwen3-coder-30b-a3b
backpack run codex-app --restore
backpack run claude-app --restore
```

Backpack does not expose a competing model-chat shell. Its local inference service exists to power coding workspaces and compatible automation.

## Apps and models

| App | Protocol | Current evidence |
| --- | --- | --- |
| Codex CLI / App | OpenAI Responses | deterministic adapter coverage; CLI has passed a real Cloud tool loop |
| Claude Code / App | Anthropic Messages | deterministic adapter coverage; Claude Code has passed a real Cloud tool loop |
| OpenCode | OpenAI-compatible Chat Completions | deterministic provider/config coverage; real-binary qualification pending |
| Pi | OpenAI-compatible Chat Completions | deterministic isolated-provider coverage; real-binary qualification pending |

The public catalog is intentionally small. Qwen3-Coder 30B A3B is the current execution-qualified coding model with experimental app compatibility. Qwen3-Coder Next remains experimental until tool use is execution-qualified. SmolLM2 is hidden behind `backpack models --all` as a fast runtime/CI fixture and is deliberately ineligible for coding workspaces.

Backpack refuses a run when trusted metadata does not declare coding, tool-calling, protocol, context, and app compatibility. Runtime execution and workspace qualification are separate claims. See [apps](docs/apps.md), [model qualification](docs/agent-model-qualification.md), and the [compatibility matrix](docs/model-compatibility.md).

## Local, SSH, and Cloud execution

```text
Coding workspace
       |
OpenAI / Anthropic protocol adapter
       |
Backpack inference + session layer
       |
Model x RuntimeAdapter x ComputeTarget
       |
llama.cpp on local / SSH / compatible managed compute
```

Backpack verifies immutable model and runtime artifacts, selects CPU/CUDA/Vulkan/Metal variants, owns sessions, reuses caches, and cleans up child processes. Advanced users can inspect operations with `backpack ps`, `backpack stop`, and `backpack runtime list`.

SSH compute is experimental:

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute doctor gpu-1
backpack run codex --model qwen3-coder-30b-a3b --compute gpu-1
```

Backpack Cloud is optional and access-controlled. Local models, SSH, and the local API require no account:

```console
backpack login
backpack cloud models
backpack run codex --model qwen3-coder-30b-a3b-instruct:cloud
```

## API compatibility

`backpack serve` exposes a loopback-only service on `127.0.0.1:11434`:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages`
- model, session, compute, health, and event endpoints under `/api/backpack/v1`

The compatibility layers focus on coding-workspace needs: streaming, roles, tools and tool results, usage, stop reasons, errors, cancellation, and qualified context limits. Unsupported semantics return explicit errors. Backpack does not claim complete OpenAI or Anthropic API parity. See [API](docs/api.md).

## Development

Development requires Go 1.24 or newer:

```console
go test ./...
go vet ./...
go build ./cmd/backpack
```

Architecture details are in [docs/architecture.md](docs/architecture.md). Source code is Apache-2.0; models and managed engines retain their own licenses. Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
