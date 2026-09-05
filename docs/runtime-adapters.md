# Runtime adapters

An adapter is selected from the normalized manifest `runtime.engine`. It validates requirements, asks the runtime manager to satisfy native/isolated dependencies, starts through a `ComputeTarget`, checks health, advertises capabilities, and stops its session. Adapters must not select behavior from model IDs or contain download URLs. Engine dependencies belong in versioned runtime requirements; reusable executable artifacts belong in the runtime catalog.

Current families are:

- `llama.cpp`: persistent native HTTP session for chat/completion.
- `whisper.cpp`: native job adapter implementing transcription without a Python dependency.
- `qwen-asr`: persistent protocol-v1 isolated-Python worker implementing transcription.
- `kokoro`: persistent protocol-v1 isolated-Python worker implementing speech synthesis.

The public server routes chat, transcription, or speech by the capability interface implemented by the selected adapter. It does not switch on an engine or model ID. Tests use fake targets/processes and protocol servers; real smoke tests determine whether a platform/runtime combination is promoted to supported.
