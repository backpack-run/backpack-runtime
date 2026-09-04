# Model compatibility

“Supported” means the pull/verify/launch/infer/stop path was exercised, not merely that a manifest parses.

| Model | Capability | Package format | Runtime adapter | Local | SSH | Status | Notes |
|---|---|---|---|---|---|---|---|
| SmolLM2 135M Instruct | chat | GGUF Q4_K_M | llama.cpp | yes | experimental | supported | Real Windows daemon/session/SSE smoke test passed 2026-09-04; SSH host test pending. |
| SmolLM2 1.7B Instruct | chat | GGUF | llama.cpp | expected | no | experimental | Manifest-compatible; large-path validation pending. |
| Qwen2.5 0.5B Instruct | chat | GGUF | llama.cpp | expected | no | experimental | Manifest-compatible. |
| Qwen3-Coder Next | chat/code | split GGUF | llama.cpp | expected | no | experimental | Split-artifact validation pending; very large. |
| Qwen3-Coder 30B A3B | chat/code | GGUF | llama.cpp | expected | no | experimental | Hardware-heavy compatibility validation pending. |
| Devstral Small 2 24B | chat/code/vision | GGUF + projector | llama.cpp | expected | no | experimental | Projector download/launch support pending. |
| Whisper Large v3 Turbo | transcription | GGML | whisper.cpp | Windows CPU bundle exists | no | runtime-required | Adapter/runtime installer pending. |
| Qwen3-ASR 0.6B | transcription | Safetensors | qwen-asr 0.0.6 | Windows CPU bundle exists | no | runtime-required | Isolated service adapter pending. |
| Kokoro 82M | speech | PyTorch | kokoro 0.9.4 | no | no | runtime-required | Validated model; attachable worker bundle still needed. |
| Z-Image-Turbo | image generation | Diffusers upstream reference | diffusers 0.40.0 | no | no | package-change-required | Metadata/static checks pass; GPU inference/output validation pending. |
| Wan2.2 TI2V 5B | video generation | Diffusers upstream reference | diffusers 0.40.0 | no | no | package-change-required | 24 GB VRAM profile; GPU inference/output validation pending. |
