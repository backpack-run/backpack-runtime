# Implementation plan

- [x] Inspect packager and Desktop contracts and document ownership/duplication.
- [x] Establish Go module, CLI/runtime separation, paths, catalog, manifest model, events, adapters, targets, and API skeleton.
- [x] Normalize explicit and legacy runtime contracts without model-name conditionals.
- [x] Pull and SHA-256 verify SmolLM2 135M.
- [x] Launch, health-check, infer, and stop SmolLM2 through llama.cpp locally.
- [ ] Make daemon-owned session persistence, `ps`, and `stop` production quality.
- [ ] Install/version llama.cpp runtime bundles and support projectors/split GGUF comprehensively.
- [ ] Validate all larger GGUF packages without putting weights in ordinary CI.
- [ ] Implement cache-aware SSH target behind the same adapter boundary.
- [ ] Implement Whisper, Qwen ASR, and Kokoro service adapters with pinned isolated runtimes.
- [ ] Implement cancellable/progress-aware image and video job adapters after packages are inference-validated.
- [ ] Publish a stable Desktop client/migration contract and add managed-compute protocol boundaries.

