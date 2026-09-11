# Codex CLI integration

`backpack launch codex` detects the real `codex` executable on `PATH`, ensures a Backpack model session, and starts Codex with a child-only Backpack provider.

The provider uses a loopback `/v1/` base URL and the Responses wire API. Backpack generates a minimal model catalog from trusted catalog/package metadata. `CODEX_HOME` points to a Backpack-owned integration directory for this process, and provider/model settings are supplied as highest-precedence command-line configuration. Normal Codex configuration, login state, history, and provider selection are not overwritten.

Apps, plugins, and multi-agent mode are disabled for the isolated launch profile. This prevents unrelated hosted discovery traffic and excludes tools that are not needed for the initial compatibility contract. Core Codex filesystem/shell tools and its normal sandbox and approval behavior remain available.

Backpack rejects passthrough arguments that could replace its managed provider, model, profile, or configuration. It never adds unsafe sandbox or approval flags.

Validated against installed `codex-cli 0.154.0-alpha.6.1` on Windows:

- isolated provider routing to a Backpack test server;
- Responses text streaming;
- an actual Codex `exec_command` invocation and tool-result continuation;
- namespace tool flattening/restoration at the protocol layer.

These tests use a deterministic fake inference backend. No catalog coding model has completed the full file-read/edit/test qualification, so model-level Codex support remains unqualified.
