# API

Public compatibility expectations are defined in [compatibility-policy.md](compatibility-policy.md). Alpha payloads may change with release notes; the `/api/backpack/v1` prefix is a routing version, not yet a beta stability promise.

Implemented loopback endpoints:

- `GET /api/backpack/v1/health`
- `GET /api/backpack/v1/version`
- `GET /api/backpack/v1/hardware`
- `GET /api/backpack/v1/models`
- `GET /api/backpack/v1/compute`
- `GET /api/backpack/v1/sessions`
- `GET /api/backpack/v1/events` (structured server-sent events)
- `GET|POST /api/backpack/v1/jobs`
- `GET|DELETE /api/backpack/v1/jobs/{id}`
- `GET /api/backpack/v1/jobs/{id}/artifacts/{artifact}`
- `POST /api/backpack/v1/sessions`
- `GET /api/backpack/v1/sessions/{id}`
- `DELETE /api/backpack/v1/sessions/{id}`
- `GET /v1/models`
- `POST /v1/chat/completions` (streaming SSE and non-streaming)
- `POST /v1/responses` (experimental Codex-oriented Responses subset)
- `POST /v1/messages` (experimental Claude Code-oriented Anthropic Messages subset)
- `POST /v1/audio/transcriptions` (multipart `file`, `model`, optional `language`, `compute`, `force`)
- `POST /v1/audio/speech` (JSON `model`, `input`, optional `voice`, `speed`, `format`, `compute`, `force`)

Job creation returns `202 Accepted`; only execution-validated runners may accept work. Job artifacts are served only after confinement and existence checks. Non-loopback binding is rejected until authentication is implemented.

Chat requests resolve catalog aliases. A ready session for the requested model is reused; otherwise the service creates a local session. Management clients choose another compute target when creating a session, after which standard chat can reuse it by model alias.

Transcription uses the same endpoint for native whisper.cpp and isolated-Python qwen-asr models. Speech currently returns `audio/wav`. API clients receive structured JSON errors; engine-specific worker endpoints remain private loopback implementation details.

OpenAI-compatible chat content arrays may include `image_url` parts. Backpack accepts inline `data:image/...` URLs only; HTTP(S) and filesystem URLs are rejected. A typed, verified `multimodal-projector` package artifact is mandatory. This contract is experimental pending real Backpack Devstral validation.

The Responses and Anthropic handlers translate through a protocol-neutral inference contract; runtime adapters do not contain client-specific wire types. See [OpenAI Responses compatibility](openai-responses-compatibility.md) and [Anthropic Messages compatibility](anthropic-compatibility.md) for the deliberately limited subsets and qualification status.
