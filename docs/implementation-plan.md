# Coding-agent runtime plan

Completed foundation:

- persistent daemon and owned model sessions
- verified model/runtime installation and managed llama.cpp variants
- local and SSH compute abstractions
- OpenAI Chat Completions, Responses, and Anthropic Messages translation
- Codex, Claude Code, Codex App, Claude App, OpenCode, and Pi launch adapters
- explicit model capabilities and per-agent protocol qualification
- Cloud routing isolated from local runtime availability

Next priorities:

1. Expand deterministic tool-loop conformance tests across every agent protocol.
2. Qualify a small set of strong coding models on representative local and remote GPUs.
3. Add explainable `--model auto` only after memory, context, quantization, and agent-quality evidence is complete.
4. Improve prompt/prefix caching and repeated-turn metrics through supported engine controls.
5. Add a trusted high-performance server runtime for user-owned GPUs without coupling protocols to that engine.

ASR, TTS, vision, image, video, and generic media jobs are intentionally not on this roadmap.
