# Anthropic Messages compatibility

Backpack implements an experimental `POST /v1/messages` subset for Claude Code. Query strings such as `?beta=true` are accepted because route matching uses the URL path.

Implemented request subset:

- string or structured system prompts;
- user/assistant text blocks;
- inline base64 image blocks;
- tool definitions;
- `tool_use` and `tool_result` continuation;
- `max_tokens`, temperature, streaming, usage, and stop reasons.

Streaming emits `message_start`, indexed content-block start/delta/stop events, text deltas, incremental tool JSON, `message_delta`, `message_stop`, and typed errors. Unknown optional request fields are ignored to tolerate compatible client metadata, while unsupported content blocks fail explicitly.

Prompt caching, token counting, hosted tools, citations, thinking blocks, batches, files, and general Anthropic API parity are not claimed. The endpoint has deterministic conformance tests but has not completed a real Claude Code session on this host.
