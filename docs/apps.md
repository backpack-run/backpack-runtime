# Coding workspace integrations

`backpack run` configures and starts an installed coding workspace while Backpack retains model, runtime, session, and compute ownership:

```console
backpack run list
backpack run doctor codex --model qwen3-coder-30b-a3b
backpack run codex --model qwen3-coder-30b-a3b
backpack run codex-app --model qwen3-coder-30b-a3b
backpack run claude --model qwen3-coder-30b-a3b
backpack run claude-app --model qwen3-coder-30b-a3b
backpack run opencode --model qwen3-coder-30b-a3b
backpack run pi --model qwen3-coder-30b-a3b
```

Backpack has no user-facing model-chat harness. The app owns prompts, repository access, tools, approvals, and the workspace UI. Backpack owns only the inference path beneath it.

## Selection and compute

The model must declare the app's required capabilities, support its wire protocol, and carry an explicit `qualified` or `compatible-experimental` app record. With no `--model`, an interactive terminal displays only eligible models; scripts must provide a model. `--model auto` is intentionally unavailable until multiple genuinely qualified choices exist.

`--compute` selects local or SSH execution. The app still talks to Backpack's authenticated loopback service, so compute location does not change its protocol configuration. `--context` may lower—but cannot exceed—the largest execution-qualified context advertised by the selected package or Cloud deployment.

Local and SSH execution need no Backpack account. Optional access-controlled Cloud models come from the live Cloud API after `backpack login`:

```console
backpack cloud models
backpack run codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack run claude --model qwen3-coder-30b-a3b-instruct:cloud
```

The `:cloud` identity is transitional. Integrations never fabricate a hosted counterpart or silently map an unavailable model.

## CLI apps

Arguments after `--` are passed literally to the external executable without a shell:

```console
backpack run codex --model qwen3-coder-30b-a3b -- --help
```

Backpack does not install external apps or add permission-bypass flags. Missing tools produce installation guidance. `--keep-alive` retains the model session after the app exits; otherwise Backpack requests a graceful stop.

Codex keeps its normal user-level `CODEX_HOME`; Backpack applies a child-only model-provider override and does not edit the CLI config. On native Windows, the child uses Codex's documented `windows.sandbox="unelevated"` fallback because current Codex builds can fail while refreshing the preferred elevated sandbox. The fallback retains ACL-based filesystem restrictions but has weaker user and network isolation.

Claude Code receives child-only Anthropic-compatible routing. OpenCode uses its supported inline provider configuration. Pi receives an isolated `PI_CODING_AGENT_DIR` containing an official custom-provider definition. The latter two do not mutate normal user configuration.

## Desktop apps

Codex App and Claude App require persistent provider profiles because the apps outlive the `backpack` command. Backpack backs up the prior profile, owns only narrowly scoped fields, refuses unsafe overwrite after unexpected drift, and provides explicit restore:

```console
backpack run codex-app --restore
backpack run claude-app --restore
```

Use `--no-open` to configure without opening the app. Quit and reopen an already-running app after changing its profile. Re-run setup after a daemon endpoint/token change.

The Codex App route merges the selected Backpack model with Codex's cached native model catalog, so signed-in OpenAI models remain available when Codex has populated that cache. Claude App's third-party gateway owns its model picker and therefore exposes the selected Backpack model through a Claude-compatible alias; it cannot simultaneously display Anthropic-hosted models in that third-party profile.

## Security boundary

Provider tokens are random per-daemon values and are redacted from diagnostics. CLI apps receive them only in the child process. Desktop apps receive an unguessable token in a Backpack-only loopback route. Cloud credentials remain in the runtime process and are never passed to third-party apps. The service remains loopback-only; it is not a remotely authenticated public API.

Backpack configures routing and model selection only. Workspace access, tool execution, sandboxing, approvals, and external-app updates remain owned by the coding app.

See the integration-specific documents and protocol subset documents for exact limitations and qualification evidence.
