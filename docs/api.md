# API

Implemented loopback endpoints:

- `GET /api/backpack/v1/health`
- `GET /api/backpack/v1/version`
- `GET /api/backpack/v1/hardware`
- `GET /api/backpack/v1/models`
- `GET /api/backpack/v1/compute`
- `GET /api/backpack/v1/sessions`
- `POST /api/backpack/v1/sessions`
- `GET /api/backpack/v1/sessions/{id}`
- `DELETE /api/backpack/v1/sessions/{id}`
- `GET /v1/models`
- `POST /v1/chat/completions` (streaming SSE and non-streaming)

Pull-progress, audio, and media job endpoints are planned. Non-loopback binding is rejected until authentication is implemented.

Chat requests resolve catalog aliases. A ready session for the requested model is reused; otherwise the service creates a local session. Management clients choose another compute target when creating a session, after which standard chat can reuse it by model alias.
