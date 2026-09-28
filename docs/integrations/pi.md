# Pi integration

```console
backpack launch doctor pi --model qwen3-coder-30b-a3b
backpack launch pi --model qwen3-coder-30b-a3b
```

Backpack discovers the installed `pi` executable and creates a launch-scoped configuration directory. Its `models.json` defines a `backpack` provider using Pi's supported `openai-completions` API mode, the authenticated loopback `/v1` base URL, the selected model, and Backpack's trusted context/output limits. The child receives the random daemon token through an environment reference; diagnostics redact it.

Arguments after `--` pass directly to Pi. Backpack does not add permission overrides and does not modify the user's normal Pi configuration. See Pi's [custom model documentation](https://pi.dev/docs/latest/models) and [configuration documentation](https://pi.dev/docs/latest/configuration).
