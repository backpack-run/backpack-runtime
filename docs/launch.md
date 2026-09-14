# Agent launch integrations

Backpack can configure and start an installed third-party coding agent while retaining model and compute ownership:

```console
backpack launch list
backpack launch doctor codex --model qwen3-coder-next
backpack launch codex --model qwen3-coder-next
backpack launch codex-app --model qwen3-coder-next
backpack launch claude --model qwen3-coder-next
backpack launch opencode --model qwen3-coder-next
```

Private-alpha Cloud models are selected with their live `:cloud` ID after `backpack login`:

```console
backpack cloud models
backpack launch codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch codex-app --model qwen3-coder-30b-a3b-instruct:cloud
```

The launch feature is experimental. The compatibility protocols and the installed Codex executable have passed isolated protocol/tool-loop tests. Qwen3-Coder 30B A3B has also passed real llama.cpp inference and a Backpack API tool-result continuation. A combined real-model Codex run loaded at 32K context, connected to Codex 0.154, and requested shell/edit tools, but this nested qualification environment enforced `read-only` and rejected execution. No security control was bypassed, so the final external-agent file/edit gate remains pending. Claude Code and OpenCode were not installed on the qualification host.

Only catalog models with an explicit `code` capability are selectable. A `tool-calling` capability is reported separately and is required before a model can be called agent-qualified. With no `--model`, an interactive terminal gets a selector; scripts must specify the model.

`--compute` selects a Backpack compute target. The external agent always talks to the local loopback Backpack service; SSH routing stays behind that API. `--context` may lower the launch context but cannot exceed package metadata. The integrations currently recommend 65,536 tokens and warn when a package declares less.

Arguments after `--` are passed literally to the external executable without a shell:

```console
backpack launch codex --model qwen3-coder-next -- --help
```

Backpack does not install external agents automatically. Missing tools produce official installation guidance. `--keep-alive` retains the model session after the agent exits; otherwise Backpack requests a graceful session stop.

Codex App is the exception to the child-only launch model. `backpack launch codex-app` writes a persistent, narrowly scoped desktop configuration and opens the installed Windows/macOS app. It accepts no passthrough arguments. Run `backpack launch codex-app --restore` to remove Backpack's managed settings and restore the preserved configuration; `--no-open` performs either operation without opening the app. Explicit reconfiguration preserves unrelated settings added by Codex or the user since the first launch. If Codex App is already running, quit and reopen it to load the change.

## Security boundary

Backpack configures provider routing, model selection, and an isolated configuration directory. It does not weaken Codex/OpenCode sandbox and approval settings or Claude Code permissions, and never adds permission-bypass flags. Tool execution, workspace access, and user approvals remain owned by the launched agent.

Provider tokens are random per-daemon values and redacted from diagnostics. CLI integrations receive them only in the child process. Codex App receives an unguessable token as part of a Backpack-only loopback route. The route sends allow-listed Backpack models to Backpack after replacing Codex authorization, while native catalog models retain their normal OpenAI/ChatGPT route. Cloud credentials remain in the runtime process and are never passed to a third-party agent. The Runtime API remains loopback-only and is not a remotely authenticated service.

See the integration-specific documents and protocol subset documents for limitations.
