# Runtime lifecycle

CLI commands health-check and reuse the persistent local service. When absent, one caller acquires the startup lock, starts the same `backpack` executable in background-service mode, records ownership, and waits for readiness. Stale locks and state are reconciled conservatively.

The service owns every model process. Sessions move through `starting`, `ready`, `stopping`, `stopped`, and `failed`; unexpected exits are detected. Graceful shutdown stops children with a forced-kill timeout. Windows child trees use kill-on-close Job Objects. Persisted PIDs are never trusted without endpoint/process reconciliation.

`backpack run` normally stops its session at exit; `--keep-alive` retains it and `--detach` returns after readiness. `backpack ps` and `backpack stop <session>` expose production lifecycle control. Compatible ready sessions are reused for repeated agent turns to avoid unnecessary model reloads.
