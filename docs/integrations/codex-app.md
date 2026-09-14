# Codex App integration

`backpack launch codex-app` configures the installed Codex desktop app on Windows or macOS to use one Backpack coding model through the local runtime service:

```console
backpack login
backpack cloud models
backpack launch codex-app --model qwen3-coder-30b-a3b-instruct:cloud
```

Local code-capable models use the same command without the `:cloud` suffix. Backpack ensures the local model session before opening Codex App.

The integration is persistent. Codex App keeps the selected model after the launch command exits, while the corresponding Backpack daemon remains available. After a reboot or daemon endpoint change, run the launch command again before using the Backpack model. If Codex App was open during configuration, quit and reopen it so the startup model catalog is reloaded.

Restore the exact configuration that existed before the first Backpack setup:

```console
backpack launch codex-app --restore
```

Use `--no-open` to configure or restore without opening the app.

## Configuration ownership

Current Codex App builds consume root-level `model`, `model_catalog_json`, and `openai_base_url` settings. Backpack patches only the managed root keys and preserves unrelated TOML settings. It does not use the older persistent `[profiles.*]` layout.

Before the first change, Backpack stores the exact original config and a versioned restore record under the Backpack config directory. Files are written privately and atomically. A later configuration or restore is refused if either the live config or saved original has drifted, preventing an automatic restore from discarding user edits. The error identifies the files that require manual reconciliation.

Backpack never reads, replaces, or deletes Codex `auth.json`. The existing Codex account remains intact.

## Routing security

The configured base URL contains an unguessable, per-daemon route token and remains bound to loopback. Codex App's existing authorization header is accepted only after that route token is verified, then replaced with Backpack's internal daemon authorization before request handling. A Backpack Cloud API key, device private key, or short-lived Cloud access token is never written to Codex configuration or passed to the app.

The token changes when a new Backpack daemon starts. Re-run `backpack launch codex-app --model ...` to refresh the persistent route after that happens.

## Compatibility status

This integration is experimental. It uses the OpenAI Responses endpoint and a trusted model catalog. Codex App may send namespace, browser, review, image, or other tools beyond the subset exercised by the Codex CLI. Cloud requests are forwarded without rewriting the tool schema; the selected model and Backpack Cloud backend must support those tools. Local model compatibility remains narrower.

Backpack does not change Codex sandbox or approval settings. Workspace access and permission prompts remain controlled by Codex App.
