# Coding-agent API

Backpack exposes a loopback-only service. The API is designed around coding-agent interoperability, not general modality coverage.

## Protocol endpoints

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages`

Chat Completions supports streaming and non-streaming text inference. Responses and Messages implement the documented subsets required by the launch integrations. Their translators cover system/developer/user/assistant/tool roles, function definitions and calls, tool results, usage, stop reasons, streaming events, and errors where the underlying model/runtime supports them.

Computer-use tools, image input, audio, speech, and media-job semantics are outside the current scope and return errors or have no route. Backpack does not fabricate tool use or structured output for a model whose trusted capabilities do not declare it.

See [Responses compatibility](openai-responses-compatibility.md) and [Messages compatibility](anthropic-compatibility.md) for exact supported shapes.

## Backpack management endpoints

- `GET /api/backpack/v1/health`
- `GET /api/backpack/v1/version`
- `GET /api/backpack/v1/hardware`
- `GET /api/backpack/v1/models`
- `GET /api/backpack/v1/cloud/models`
- `GET /api/backpack/v1/compute`
- `GET|POST /api/backpack/v1/sessions`
- `GET|DELETE /api/backpack/v1/sessions/{id}`
- `GET /api/backpack/v1/events`
- `POST /api/backpack/v1/shutdown`

Requests reuse a compatible ready session or create one through `Model x RuntimeAdapter x ComputeTarget`. Structured events describe model/runtime preparation, session state, downloads, and failures; they contain data rather than terminal formatting.

The server rejects non-loopback binds. Launch integrations use random per-daemon bearer credentials; upstream Cloud credentials remain inside Backpack and are not passed to agents.
