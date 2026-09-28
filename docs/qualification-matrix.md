# Qualification matrix

Claims are promoted independently for runtime execution, protocol behavior, agent integration, compute target, and operating system.

| Area | Windows amd64 | Linux amd64 | macOS arm64 |
| --- | --- | --- | --- |
| llama.cpp archive/build | qualified | qualified archive; CPU smoke evidence | archive only |
| SmolLM2 runtime fixture | qualified | qualified CPU smoke | unqualified |
| Qwen3-Coder 30B A3B | qualified text/tool-result continuation where recorded | unqualified | unqualified |
| SSH lifecycle | deterministic transport coverage; real host still experimental | remote target | remote target |
| Codex / Claude adapters | deterministic coverage; prior Cloud tool-loop evidence | deterministic coverage | deterministic coverage |
| OpenCode / Pi adapters | deterministic configuration coverage | deterministic coverage | deterministic coverage |
| Codex App / Claude App | deterministic config/restore coverage; experimental | not applicable | experimental, real session unqualified |

A build or parse success is not execution qualification. Agent qualification additionally requires usable context, tools, protocol translation, cancellation/error behavior, and a real or meaningful deterministic tool loop.
