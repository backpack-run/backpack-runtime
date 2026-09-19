# Claude App integration

`backpack launch claude-app` (also accepted as `claude-desktop`) configures Claude App's third-party inference gateway mode for one trusted Backpack coding model:

```console
backpack login
backpack cloud models
backpack launch claude-app --model qwen3-coder-30b-a3b-instruct:cloud
```

Local code-capable models use the same command without `:cloud`; Backpack creates and retains the required local model session. Use `--no-open` to configure without launching the app.

The managed gateway is loopback-only. Its URL contains a random per-daemon token and the selected model's execution-qualified context limit. Claude sees a fixed compatibility model ID; Backpack validates and rewrites it to the selected catalog model. The gateway provides model discovery, conservative token-count estimation for compaction planning, and the Anthropic Messages endpoint. The Cloud credential never enters Claude's configuration or process.

Backpack saves the exact original profile files in private state before changing them. Restore them with:

```console
backpack launch claude-app --restore
```

Restoration refuses to overwrite files that changed unexpectedly after setup. Re-running the launch command safely changes the selected model only while the managed files still match Backpack's recorded state.

This integration is experimental. Its gateway, model rewrite, authentication, and restore behavior have deterministic tests, but a real Claude App session was not available on the qualification host. Claude App must be installed separately. If it is already open, quit and reopen it after configuration. Native Anthropic models belong to Claude's normal first-party profile; restoring switches back to that profile.
