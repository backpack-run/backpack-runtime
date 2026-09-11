# Models and compatibility

`backpack models` lists every package in the trusted Backpack catalog with its public alias, capabilities, support status, installed marker, and locally known package size and fit. Package size and fit are shown after installation; `backpack model show <model>` resolves trusted package metadata and provides the detailed pre-download fit report.

```console
backpack models
backpack models --json
backpack model show qwen3-coder-30b-a3b
backpack pull smollm2-135m
backpack list
```

`backpack models --json` is the stable automation form. `backpack list` is an installed-package inventory and may include local package directories; it is intended for local administration rather than sanitized bug reports.

Catalog status has a deliberately strict meaning:

- `supported`: real package install and execution completed on at least one documented platform.
- `experimental`: an implementation or package contract exists, but required execution qualification is incomplete.
- `preflight-only`: Backpack can inspect and refuse or route the package safely, but cannot claim execution.
- `package-change-required`: the published package lacks a complete executable contract.
- `unsupported`: the package is known and intentionally unavailable.

Run `backpack catalog verify` to reconcile the trusted catalog with the live `backpack-run` Hugging Face organization. The networked CI check detects missing packages, nonexistent catalog repositories, invalid runtime metadata, schema incompatibility, and duplicate aliases without promoting package existence to support.

The complete evidence and per-platform caveats are maintained in [model-compatibility.md](model-compatibility.md) and [qualification-matrix.md](qualification-matrix.md).
