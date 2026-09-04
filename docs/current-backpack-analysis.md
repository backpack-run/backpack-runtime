# Current Backpack analysis

Reviewed 2026-09-04 against the local `backpack-model-packager` and Backpack Desktop repositories, plus the published Backpack manifests used by the catalog.

## Package contract

Published packages use `backpack-model.yaml` schema version 1. A logical `model` and immutable `upstream` source contain one or more concrete `packages`. Each package contains artifact identity, format/precision, file inventory, SHA-256 values, memory estimates, validation results, provenance, and runtime metadata. GGUF artifacts name `llama.cpp` and a tested revision. Whisper names `whisper.cpp`; Qwen ASR carries `qwen-asr==0.0.6`; Kokoro carries `kokoro==0.9.4` plus its language dependency. Qwen and Whisper also publish checked runtime-service descriptors and protocol-v1 bundles. Media packages use a `pipeline` contract with pinned Diffusers dependencies and immutable component inventories; their published Backpack repositories are lightweight upstream references rather than complete local weight copies.

The runtime must preserve schema-v1 compatibility. The scalable extension is a normalized requirement:

```yaml
runtime:
  engine: qwen-asr
  version: "0.0.6"
  environment: isolated-python
```

This repository accepts that explicit form and otherwise normalizes legacy package runtime fields by engine, never by model ID.

## Desktop runtime ownership today

`src-tauri/src/lib.rs` currently owns model URLs/checksums/revisions, downloads, cache paths, llama.cpp installation and process lifecycle, local hardware probing, Whisper installation/execution, Qwen/Kokoro isolated environments, media package resolution/workers, and SSH orchestration. Runtime state is in process mutexes. Desktop uses its Tauri local-data directory, typically `%LOCALAPPDATA%/Backpack`, with `engine`, `models`, `voice`, `runtimes`, `media`, and logs beneath it.

Local llama.cpp starts `llama-server.exe` on loopback with model, context, GPU layer, alias, and optional projector arguments. Desktop downloads to `.part`, hashes content, then renames. Hardware uses `sysinfo` plus Windows CIM GPU inspection. Its SSH path validates identity components, uses OpenSSH with keepalive and host-key acceptance, probes Linux/CUDA/GPU/disk state, caches under `~/.cache/backpack`, bootstraps llama.cpp, syncs artifacts, opens a loopback tunnel, and tracks disconnection/recovery state.

## Duplication to remove over time

Catalog data exists in Desktop TypeScript while package truth exists in manifests. Voice install recipes and hashes are hard-coded Rust constants. Engine, voice, media, and remote lifecycle paths use separate state machines. Hardware and checksum logic are Desktop-specific. These should move behind Runtime APIs; conversations, UI preferences, rendering, and workflows should remain in Desktop.

## Execution findings

- GGUF text models: llama.cpp; SmolLM2 135M is the fast end-to-end test package.
- Whisper: published Windows x64 CPU whisper.cpp service; CUDA/Vulkan/Metal are catalog-only.
- Qwen3-ASR: published Windows x64 pinned qwen-asr 0.0.6/PyTorch CPU service.
- Kokoro: model validates with Kokoro 0.9.4, but no attachable runtime-service bundle is published.
- Z-Image and Wan: pinned Diffusers 0.40.0 pipeline metadata and upstream component checksums exist. Output inference validation is skipped; full upstream weights are not embedded in the Backpack package.

