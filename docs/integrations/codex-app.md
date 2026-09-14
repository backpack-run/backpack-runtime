# Codex App integration

`backpack launch codex-app` configures the installed Codex desktop app on Windows or macOS to add one Backpack coding model to Codex's native model picker through the local runtime service:

```console
backpack login
backpack cloud models
backpack launch codex-app --model qwen3-coder-30b-a3b-instruct:cloud
```

Local code-capable models use the same command without the `:cloud` suffix. Backpack ensures the local model session before opening Codex App.

The integration is persistent. Codex App keeps the selected model after the launch command exits, while the corresponding Backpack daemon remains available. After a reboot or daemon endpoint change, run the launch command again before using the Backpack model. If Codex App was open during configuration, quit and reopen it so the startup model catalog is reloaded.

Backpack reads Codex's model metadata cache and combines those entries with the selected Backpack model. It does not hard-code OpenAI model names: the native entries shown depend on the installed Codex version, the signed-in account, entitlements, and cached catalog. If Codex has not populated that cache yet, open Codex normally once and then repeat the launch command.

Restore the configuration that existed before Backpack setup, including any unrelated changes preserved by an explicit later reconfiguration:

```console
backpack launch codex-app --restore
```

Use `--no-open` to configure or restore without opening the app.

## Configuration ownership

Current Codex App builds consume root-level `model`, `model_catalog_json`, and `openai_base_url` settings. Backpack patches only the managed root keys and preserves unrelated TOML settings. It does not use the older persistent `[profiles.*]` layout.

Before the first change, Backpack stores the exact original config and a versioned restore record under the Backpack config directory. Files are written privately and atomically. A direct restore is refused if the live config or saved original has drifted, preventing an automatic restore from discarding user edits. Explicitly running `launch codex-app` again refreshes only Backpack's managed keys and rebases the restore copy on the current unrelated settings; this accommodates Codex upgrades without rolling those settings back.

Backpack never reads, replaces, or deletes Codex `auth.json`. The existing Codex account remains intact.

## Routing security

The configured base URL contains an unguessable, per-daemon route token and remains bound to loopback. After verifying that token, the router uses an independent trusted Backpack model allow-list. For a Backpack model it replaces Codex App's authorization header before request handling, so OpenAI credentials never reach Backpack Cloud. For a native model it forwards the existing authorization only to the fixed OpenAI API or ChatGPT HTTPS origin. Redirects are rejected. A Backpack Cloud API key, device private key, or short-lived Cloud access token is never written to Codex configuration or passed to the app.

The token changes when a new Backpack daemon starts. Re-run `backpack launch codex-app --model ...` to refresh the persistent route after that happens.

## Compatibility status

This integration is experimental. It uses the OpenAI Responses endpoint and a trusted model catalog. Codex App may send namespace, browser, review, image, or other tools beyond the subset exercised by the Codex CLI. Cloud requests are forwarded without rewriting the tool schema; the selected model and Backpack Cloud backend must support those tools. Local model compatibility remains narrower.

Backpack does not change Codex sandbox or approval settings. Workspace access and permission prompts remain controlled by Codex App.
