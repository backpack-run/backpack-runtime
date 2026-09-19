# Agent launch integrations

Backpack can configure and start an installed third-party coding agent while retaining model and compute ownership:

```console
backpack launch list
backpack launch doctor codex --model qwen3-coder-next
backpack launch codex --model qwen3-coder-next
backpack launch codex-app --model qwen3-coder-next
backpack launch claude --model qwen3-coder-next
backpack launch claude-app --model qwen3-coder-next
backpack launch opencode --model qwen3-coder-next
```

Private-alpha Cloud models are selected with their live `:cloud` ID after `backpack login`:

```console
backpack cloud models
backpack launch codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch codex-app --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch claude --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch claude-app --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch opencode --model qwen3-coder-30b-a3b-instruct:cloud
```

The launch feature is experimental. The compatibility protocols and the installed Codex executable have passed isolated protocol/tool-loop tests. Qwen3-Coder 30B A3B has also passed real llama.cpp inference and a Backpack API tool-result continuation. A combined real-model Codex run loaded at 32K context, connected to Codex 0.154, and requested shell/edit tools, but this nested qualification environment enforced `read-only` and rejected execution. No security control was bypassed, so the final external-agent file/edit gate remains pending. Claude Code and OpenCode were not installed on the qualification host.

Only catalog models with an explicit `code` capability are selectable. A `tool-calling` capability is reported separately and is required before a model can be called agent-qualified. With no `--model`, an interactive terminal gets a selector; scripts must specify the model.

`--compute` selects a Backpack compute target. The external agent always talks to the local loopback Backpack service; SSH routing stays behind that API. By default Backpack advertises the largest execution-qualified context declared by the selected package or Cloud deployment. `--context` may lower it but cannot exceed that trusted limit. A model's larger theoretical/native window is not advertised until the actual backend has qualified it. Claude Code receives the same value as its automatic compaction boundary.

Arguments after `--` are passed literally to the external executable without a shell:

```console
backpack launch codex --model qwen3-coder-next -- --help
```

Backpack does not install external agents automatically. Missing tools produce official installation guidance. `--keep-alive` retains the model session after the agent exits; otherwise Backpack requests a graceful session stop.

Codex keeps its normal user-level `CODEX_HOME`; Backpack applies provider routing with higher-precedence command-line overrides and never edits the user's CLI config. This avoids the misleading “unsupported project-local config keys” warning that occurred when an older Backpack launch isolated `CODEX_HOME` while the working directory was the user's home folder.

Codex App and Claude App are exceptions to the child-only launch model. Their launch commands write persistent, narrowly scoped desktop profiles and open the installed Windows/macOS app. They accept no passthrough arguments. Use `backpack launch codex-app --restore` or `backpack launch claude-app --restore` to restore the previous configuration; `--no-open` changes configuration without opening the app. If an app is already running, quit and reopen it to load the change.

Claude App support uses its third-party inference gateway mode and a dedicated authenticated loopback route. It advertises only the selected Backpack model, implements the app's model and token-count discovery calls, and rewrites the Claude-facing compatibility alias to the trusted Backpack model ID. The original first-party profile files are retained in private restore state and restoration is refused after unexpected config drift. This path is experimental and has deterministic gateway tests; a real Claude App session was not available on the qualification host.

## Security boundary

Backpack configures provider routing, model selection, and an isolated configuration directory. It does not weaken Codex/OpenCode sandbox and approval settings or Claude Code permissions, and never adds permission-bypass flags. Tool execution, workspace access, and user approvals remain owned by the launched agent.

Provider tokens are random per-daemon values and redacted from diagnostics. CLI integrations receive them only in the child process. Desktop integrations receive an unguessable token as part of a Backpack-only loopback route. The Codex route sends allow-listed Backpack models to Backpack after replacing Codex authorization, while native catalog models retain their normal OpenAI/ChatGPT route. The Claude route accepts only its fixed compatibility model alias and rewrites it to the selected trusted Backpack model. Cloud credentials remain in the runtime process and are never passed to a third-party agent. The Runtime API remains loopback-only and is not a remotely authenticated service.

See the integration-specific documents and protocol subset documents for limitations.
