# Runtime lifecycle

Ordinary lifecycle commands check persisted endpoint health and reuse the service when alive. Otherwise one caller wins an exclusive startup file, chooses the default or a free loopback port, starts the same `backpack` executable in background-service mode, writes PID/endpoint/start-time metadata, and waits for health. Contending callers wait; an aged startup lock is recoverable.

The service owns adapter process handles. On Windows each child tree is assigned to a kill-on-close Job Object, so an abrupt service exit terminates inference children. Native servers and persistent Python workers use sessions that move through `starting`, `ready`, `stopping`, `stopped`, and `failed`. Python workers are launched beneath managed uv so the tracked parent remains alive across the venv redirector boundary. Metadata is atomically persisted without process handles. Child exit is monitored and unexpected exit becomes `failed`. On graceful service shutdown all owned sessions receive graceful stop with a forced-kill timeout. After a service crash, previously active sessions are marked failed rather than trusted from PID alone.

Whisper transcription is deliberately job-scoped: the daemon owns the command until completion or cancellation, but does not invent an idle server session for an engine whose native interface is a CLI job. Qwen3-ASR and Kokoro keep loaded workers as sessions because model load cost is material; subsequent API calls reuse a compatible ready session.

`backpack run` creates an API-owned session. Normal interactive or one-shot exit stops it; `--keep-alive` retains it, and `--detach` creates it and returns. `backpack ps` shows active sessions and `backpack stop <session>` stops one exact session.
