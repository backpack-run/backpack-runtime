# Agent model qualification

Backpack treats model selection as admission control, not name matching.

## Facts recorded per model

- immutable package and runtime identity
- coding, tool-calling, reasoning, and structured-output capabilities when verified
- trusted context window actually deployable by the selected backend
- supported wire protocols
- memory/VRAM and quantization metadata
- per-agent compatibility status, protocol, and limitation reason

Capability facts are distinct from benchmark or quality evidence. Backpack does not invent benchmark scores.

## Admission rule

An agent descriptor declares its protocol and required capabilities. Launch is permitted only when the model declares every required capability and its per-agent entry is `qualified` or `compatible-experimental` for that same protocol. `untested`, `incompatible`, absent, or protocol-mismatched entries are refused with an actionable reason.

SmolLM2 illustrates the distinction: it can validate the llama.cpp lifecycle but is intentionally not eligible for a coding workspace. Qwen3-Coder Next is a coding model, but remains ineligible until tool calling is execution-qualified.

## Toward `--model auto`

Automatic selection must rank only admitted models, then account for compute target, RAM/VRAM, quantization, runtime availability, requested context, and validation strength. The selection must print why a model was chosen and which constraints excluded alternatives. The current release establishes the metadata/interface but intentionally does not make a low-evidence automatic choice.
