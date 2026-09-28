# Coding-model compatibility

`supported` means the model's package and text inference path have real execution evidence on at least one documented platform. Agent compatibility is a separate, stricter claim.

| Model | Package/runtime | Text execution | Agent compatibility | Status |
| --- | --- | --- | --- | --- |
| Qwen3-Coder 30B A3B Instruct | GGUF / managed llama.cpp | Windows-qualified; large hardware requirement | Codex, Claude, OpenCode, and Pi are `compatible-experimental`; tool-result continuation is covered | supported runtime, experimental agents |
| Qwen3-Coder Next | split GGUF / managed llama.cpp | package-contract coverage only | not eligible: tool calling has not been execution-qualified | experimental |
| SmolLM2 135M Instruct | GGUF / managed llama.cpp | Windows/Linux smoke fixture | deliberately ineligible for agent launch | internal test fixture |

Models removed from this catalog are not implied unsupported by llama.cpp; they are outside Backpack's curated coding-agent product or lack sufficient evidence. The catalog does not include generic chat, ASR, TTS, vision, image, or video packages.

Run `backpack models` for the public coding catalog and `backpack models --all` to include internal runtime fixtures. `backpack launch list` reports the number of models eligible for each agent.
