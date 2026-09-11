# OpenAI Responses compatibility

Backpack implements `POST /v1/responses` as an experimental compatibility endpoint for Codex CLI. It translates into Backpack's protocol-neutral inference request and then into the selected runtime adapter's chat/tool interface.

Implemented request subset:

- string input and message-item arrays;
- system/developer instructions;
- inline text and `data:image/...` content;
- streaming and non-streaming output;
- `max_output_tokens` and temperature;
- function definitions, function calls, and function-call outputs;
- namespace containers flattened as `<namespace>__<function>` for chat backends and restored on Responses output;
- usage and terminal completion/error state.

Provider-executed `web_search` declarations are intentionally omitted because Backpack cannot safely convert them into client functions. Other unsupported tool/content/input types return structured errors. Reasoning items, hosted tools, conversations, background mode, compaction, and general Responses API parity are not claimed.

Streaming uses typed Responses SSE events for response creation, output items, text deltas, function argument deltas, completion, and failure. The implementation has passed a real installed Codex text stream and an actual Codex shell-tool round trip against a deterministic local inference fixture. That validates the client/protocol path, not any individual model's tool-call quality.
