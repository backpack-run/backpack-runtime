# Model-fit policy

Backpack estimates fit from package metadata and detected compute hardware. The result is advisory—it cannot account perfectly for context growth, driver allocations, other processes, or every engine—but it prevents clearly harmful silent CPU fallback.

- `excellent`: the estimated accelerator requirement fits detected VRAM.
- `good`: the package fits system memory and CPU execution is considered reasonable.
- `constrained`: it likely fits, but recommended memory leaves little operating-system/context headroom.
- `remote-recommended`: accelerator-oriented execution does not fit detected VRAM and CPU fallback is likely impractical.
- `unsupported`: the estimated working set exceeds a safe share of system RAM.

`backpack inspect <installed-model>` includes the local report. Session creation, transcription, and speech refuse `remote-recommended` or `unsupported` fits. CLI commands accept `--force` as an explicit override; clients pass `options.force` for sessions or `force` on audio requests. The policy uses manifest RAM/VRAM estimates and artifact size, never a model-name table.
