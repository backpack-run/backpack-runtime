# Coding models

`backpack models` lists the curated public coding catalog. `backpack models --json` adds trusted protocols, context, and compatible-agent metadata for automation. `--all` additionally exposes internal runtime fixtures.

```console
backpack models
backpack models show qwen3-coder-30b-a3b
backpack pull qwen3-coder-30b-a3b
backpack models installed
```

Runtime support and app qualification are separate. A model may execute text correctly yet remain unavailable to `backpack run <app>` until coding, tool-calling, protocol, context, and app-specific behavior are qualified. See [model compatibility](model-compatibility.md) and [agent qualification](agent-model-qualification.md).

`backpack models verify` checks trusted entries against their immutable public packages where network access is allowed. Catalog size is not a product goal.
