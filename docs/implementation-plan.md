# Implementation plan

- [x] Inspect packager and Desktop contracts and document ownership/duplication.
- [x] Establish Go module, CLI/runtime separation, paths, catalog, manifest model, events, adapters, targets, and API skeleton.
- [x] Normalize explicit and legacy runtime contracts without model-name conditionals.
- [x] Pull and SHA-256 verify SmolLM2 135M.
- [x] Launch, health-check, infer, and stop SmolLM2 through llama.cpp locally.
- [x] Add auto-started service ownership, persisted sessions, `ps`, exact-session `stop`, child monitoring, and SSE proxying.
- [x] Install/version verified llama.cpp runtime bundles locally and transfer them to SSH caches.
- [ ] Support projectors/split GGUF comprehensively.
- [ ] Validate all larger GGUF packages without putting weights in ordinary CI.
- [x] Add SSH configuration, probing, checksum-aware model/runtime sync, remote launch, and loopback tunneling (real-host smoke test pending).
- [ ] Implement Whisper, Qwen ASR, and Kokoro service adapters with pinned isolated runtimes.
- [ ] Implement cancellable/progress-aware image and video job adapters after packages are inference-validated.
- [ ] Publish a stable Desktop client/migration contract and add managed-compute protocol boundaries.
