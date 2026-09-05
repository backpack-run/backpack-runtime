# Implementation plan

- [x] Inspect packager and Desktop contracts and document ownership/duplication.
- [x] Establish Go module, CLI/runtime separation, paths, catalog, manifest model, events, adapters, targets, and API skeleton.
- [x] Normalize explicit and legacy runtime contracts without model-name conditionals.
- [x] Pull and SHA-256 verify SmolLM2 135M.
- [x] Launch, health-check, infer, and stop SmolLM2 through llama.cpp locally.
- [x] Add auto-started service ownership, persisted sessions, `ps`, exact-session `stop`, child monitoring, and SSE proxying.
- [x] Install/version verified llama.cpp runtime bundles locally and transfer them to SSH caches.
- [x] Validate complete split-GGUF sets and pass typed projector artifacts to llama.cpp without model-name conditionals.
- [x] Add metadata-only resolution and fit refusal for larger GGUF packages without putting weights in ordinary CI (real large-model inference remains pending).
- [x] Add SSH configuration, probing, checksum-aware model/runtime sync, remote launch, and loopback tunneling (real-host smoke test pending).
- [x] Implement and real-smoke-test Windows CPU Whisper transcription through a verified managed native runtime.
- [x] Implement the managed uv/Python environment boundary, protocol-v1 worker client, and Qwen ASR/Kokoro adapters and APIs.
- [x] Complete real clean-environment Kokoro synthesis, cache reuse, managed session stop, and no-orphan validation.
- [x] Complete real clean-environment Qwen ASR pull, transcription, cache/session reuse, and stop/no-orphan validation.
- [x] Add manifest-based model-fit classification and explicit `--force` override for poor local fallback.
- [x] Add persistent cancellable jobs, structured progress, confined output artifacts, HTTP routes, CLI surfaces, and public Go client types.
- [ ] Register image/video runners only after immutable package contracts and real GPU execution are validated.
- [x] Extend the generic public client/event contract; managed Backpack Compute remains out of scope.
