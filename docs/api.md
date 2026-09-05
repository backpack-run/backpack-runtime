# API

Implemented loopback endpoints:

- `GET /api/backpack/v1/health`
- `GET /api/backpack/v1/version`
- `GET /api/backpack/v1/hardware`
- `GET /api/backpack/v1/models`
- `GET /api/backpack/v1/compute`
- `GET /api/backpack/v1/sessions`
- `GET /api/backpack/v1/events` (structured server-sent events)
- `POST /api/backpack/v1/sessions`
- `GET /api/backpack/v1/sessions/{id}`
- `DELETE /api/backpack/v1/sessions/{id}`
- `GET /v1/models`
- `POST /v1/chat/completions` (streaming SSE and non-streaming)
- `POST /v1/audio/transcriptions` (multipart `file`, `model`, optional `language`, `compute`, `force`)
- `POST /v1/audio/speech` (JSON `model`, `input`, optional `voice`, `speed`, `format`, `compute`, `force`)

Pull-progress and media job endpoints are planned. Non-loopback binding is rejected until authentication is implemented.

Chat requests resolve catalog aliases. A ready session for the requested model is reused; otherwise the service creates a local session. Management clients choose another compute target when creating a session, after which standard chat can reuse it by model alias.

Transcription uses the same endpoint for native whisper.cpp and isolated-Python qwen-asr models. Speech currently returns `audio/wav`. API clients receive structured JSON errors; engine-specific worker endpoints remain private loopback implementation details.
