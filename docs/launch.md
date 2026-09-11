# Agent launch integrations

Backpack can configure and start an installed third-party coding agent while retaining model and compute ownership:

```console
backpack launch list
backpack launch doctor codex --model qwen3-coder-next
backpack launch codex --model qwen3-coder-next
backpack launch claude --model qwen3-coder-next
backpack launch opencode --model qwen3-coder-next
```

The launch feature is experimental. The compatibility protocols and the installed Codex executable have passed isolated protocol/tool-loop tests, but no Backpack coding model has yet completed the full real-model agent qualification. Claude Code and OpenCode were not installed on the qualification host.

Only catalog models with an explicit `code` capability are selectable. A `tool-calling` capability is reported separately and is required before a model can be called agent-qualified. With no `--model`, an interactive terminal gets a selector; scripts must specify the model.

`--compute` selects a Backpack compute target. The external agent always talks to the local loopback Backpack service; SSH routing stays behind that API. `--context` may lower the launch context but cannot exceed package metadata. The integrations currently recommend 65,536 tokens and warn when a package declares less.

Arguments after `--` are passed literally to the external executable without a shell:

```console
backpack launch codex --model qwen3-coder-next -- --help
```

Backpack does not install external agents automatically. Missing tools produce official installation guidance. `--keep-alive` retains the model session after the agent exits; otherwise Backpack requests a graceful session stop.

## Security boundary

Backpack configures provider routing, model selection, and an isolated configuration directory. It does not weaken Codex/OpenCode sandbox and approval settings or Claude Code permissions, and never adds permission-bypass flags. Tool execution, workspace access, and user approvals remain owned by the launched agent.

Provider tokens used for loopback compatibility are placeholders scoped to the child process and are redacted from diagnostics. The Runtime API remains loopback-only because it has no remote-client authentication.

See the integration-specific documents and protocol subset documents for limitations.
