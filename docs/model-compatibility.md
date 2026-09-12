# Model compatibility

"Supported" means the pull/verify/launch/infer/stop path was exercised, not merely that a manifest parses.

Agent qualification is stricter: the package must declare code and tool-calling capability and complete a real external-agent file/tool session. Qwen3-Coder 30B A3B now has a trusted `tool-calling` declaration after a real two-stage Backpack API tool loop, but no model is Codex- or Claude-qualified until an external-agent file/tool session passes. The Responses adapter itself has passed an installed Codex shell-tool round trip using a deterministic inference fixture.

| Model | Capability | Package format | Runtime adapter | Local | SSH | Status | Notes |
|---|---|---|---|---|---|---|---|
| SmolLM2 135M Instruct | chat | GGUF Q4_K_M | managed llama.cpp | Windows x64 and Linux amd64 CPU | experimental | supported | Real Windows daemon/session/SSE smoke and clean Linux release-archive pull/infer/detach/stop qualification passed. SSH host test pending. |
| SmolLM2 1.7B Instruct | chat | GGUF Q4_K_M | managed llama.cpp | Windows x64 CPU | experimental | supported | Real pull/verify/launch/infer/stop returned `BACKPACK_SMOL_17_OK`. SSH host test pending. |
| Qwen2.5 0.5B Instruct | chat | GGUF Q4_K_M | managed llama.cpp | Windows x64 CPU | experimental | supported | Real clean-home test returned `BACKPACK_QWEN_OK`; one-byte corruption was detected, repaired, and re-verified. |
| Qwen3-Coder Next | chat/code | 4-part split GGUF | managed llama.cpp | contract only | contract only | experimental | All shard names/hashes are required; shard 1 is the entrypoint. Missing-shard tests pass. No 48.4 GB real download on this host. |
| Qwen3-Coder 30B A3B | chat/code/tool calling | GGUF Q4_K_M | managed llama.cpp | Windows x64 CPU with `--force` | contract only | supported | An 18.56 GB pull survived three real connection resets through exact-range resume, verified SHA-256, and atomically installed. llama.cpp b10618 loaded the package, returned exact inference markers, completed a structured function call and tool-result continuation, and streamed incremental tool arguments through SSE. The normal fit policy still recommends remote compute on this 31.6 GB integrated-GPU host. External-agent qualification remains pending. |
| Devstral Small 2 24B | chat/code/vision | GGUF + typed projector | managed llama.cpp/libmtmd | contract only | contract only | experimental | Projector is downloaded, verified, synced, and passed as `--mmproj`; inline OpenAI image content is accepted. No runtime vision claim until a real model/projector test succeeds. |
| GLM-5.3 Flash | image/text-to-text | single GGUF Q4_K_M + F16 projector | llama.cpp/libmtmd contract | preflight only | contract only | experimental | 321B MoE; 193.8 GB primary plus 1.13 GB projector. Package integrity passed but load failed on its recorded development revision; managed llama.cpp b10618 is incompatible. Consumer hardware is refused before download. No tools/code or vision execution claim. |
| Whisper Large v3 Turbo | transcription | GGML | managed whisper.cpp | Windows x64 CPU | unavailable | supported | Clean-home pull/runtime-install/transcribe test passed with the pinned JFK fixture. Other platform bundles have not been published. |
| Qwen3-ASR 0.6B | transcription | Safetensors | isolated Python / qwen-asr 0.0.6 | Windows x64 CPU | unavailable | supported | Schema-v3 SHA-256 lock rebuilt 95 hashed distributions; ISO `en` normalized; JFK transcription passed. |
| Kokoro 82M | speech | PyTorch | isolated Python / kokoro 0.9.4 | Windows x64 CPU | unavailable | supported | Schema-v3 hash-locked rebuild and real WAV synthesis passed; RIFF/WAVE output validated. |
| Z-Image-Turbo | image generation | hybrid: Backpack INT8 transformer + immutable upstream components | diffusers 0.40.0 | no | no | package-change-required | Contract pins transformer/text encoder/tokenizer/VAE/scheduler and about 32.8 GB upstream bytes. No complete executable package or GPU output validation. |
| Wan2.2 TI2V 5B | text/image-to-video | hybrid: Backpack INT8 transformer + immutable Diffusers components | diffusers 0.40.0 | no | no | package-change-required | Contract declares both TI2V tasks and a 24 GB VRAM profile. No complete executable package or GPU output validation. |

## Coding-agent qualification

| Model | Inference | Code declared | Tool calling declared | Codex | Claude Code |
|---|---|---|---|---|---|
| Qwen3-Coder Next | contract only | yes | no | unqualified | unqualified |
| Qwen3-Coder 30B A3B | Windows CPU chat/tool loop passed | yes | yes | unqualified: real CLI/model connected and requested tools, but host policy blocked execution/edit | unqualified |
| Devstral Small 2 24B | contract only | yes | no | unqualified | unqualified |
| GLM-5.3 Flash | preflight only | no | no | not offered | not offered |
| SmolLM2 / Qwen2.5 small models | validated chat | no | no | not offered | not offered |
