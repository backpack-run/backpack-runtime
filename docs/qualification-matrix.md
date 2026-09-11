# Release qualification matrix

This matrix distinguishes execution evidence from build or contract checks. A green build is not an inference qualification, and a parsed manifest is not model support.

## Platform matrix

| Check | Windows amd64 | Linux amd64 | macOS arm64 |
|---|---|---|---|
| Install from public `v0.1.0-alpha.1` asset | passed | passed | archive only |
| Version and sanitized doctor | passed | passed | build only |
| Catalog and model pull | passed | passed | untested |
| Managed llama.cpp and SmolLM2 chat | passed | passed | untested |
| Daemon, detach, `ps`, stop, cleanup | passed | passed | untested |
| Corruption detection/recovery | passed | deterministic tests | deterministic tests |
| Whisper | passed | unqualified | unqualified |
| Qwen3-ASR | passed | unqualified | unqualified |
| Kokoro | passed | unqualified | unqualified |
| Upgrade from previous release | implementation/tests | implementation/tests | implementation/tests |
| Uninstall/reinstall | passed manually | documented | documented |

Windows is the supported alpha platform. Linux remains an experimental preview despite the real CPU GGUF qualification. macOS arm64 remains a build/archive preview until an Apple Silicon host completes managed-runtime inference.

## Agent and compute matrix

| Check | Evidence | Status |
|---|---|---|
| OpenAI Responses text/tool protocol | installed Codex CLI plus deterministic fake inference backend | passed |
| Codex shell-tool round trip | real Codex binary executed a shell tool and continued from its output | protocol-qualified |
| Codex with a real Backpack coding model | requires compatible model/runtime and practical hardware | unqualified |
| Claude Messages text/tool protocol | deterministic conformance tests | protocol-qualified |
| Claude Code real binary | executable unavailable on qualification host | unqualified |
| OpenCode provider/config isolation | deterministic invocation tests | implementation-qualified |
| OpenCode real binary/model | executable unavailable on qualification host | unqualified |
| SSH command, host-key, sync and tunnel behavior | deterministic unit/integration abstractions | implementation-qualified |
| Real SSH model inference | no configured SSH target or credentials | unqualified |

## Large and generated-media matrix

| Family | Contract evidence | Execution evidence | Status |
|---|---|---|---|
| Single GGUF | checksum/install/session coverage | multiple real Windows models; SmolLM2 on Linux | supported where listed |
| Split GGUF | complete-shard and missing-shard tests | none on this host | experimental |
| Projector/vision | typed auxiliary artifact and `--mmproj` coverage | none on this host | experimental |
| Large/MoE GGUF | manifest selection and fit refusal | none on suitable hardware | preflight only |
| Z-Image | immutable component inventory and job/artifact API | no complete package or GPU generation | package change required |
| Wan video | immutable component inventory and job/artifact API | no complete package or GPU generation | package change required |

## Reproduction

Lightweight release gates:

```console
gofmt -w .
go test ./...
go vet ./...
go build ./cmd/backpack
git diff --check
goreleaser release --snapshot --clean
```

Network-, platform-, agent-, SSH-, and GPU-dependent qualification belongs in explicit opt-in workflows. [Extended release qualification](../.github/workflows/extended-qualification.yml) provides self-hosted macOS arm64, real Codex/model, and real SSH gates against an exact immutable release. Each job is disabled by default and requires its declared runner labels, tools, hardware, and secrets; absence is an unavailable gate, never inferred success.

See [model compatibility](model-compatibility.md), [launch integrations](launch.md), [release readiness](release-readiness.md), and [SSH compute](ssh-compute.md) for the associated gates.
