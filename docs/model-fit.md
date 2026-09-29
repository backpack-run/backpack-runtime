# Model-fit policy

Backpack compares trusted package estimates and artifact size with detected RAM/VRAM before download or model start:

- `excellent`: accelerator requirement fits detected VRAM.
- `good`: system memory and CPU execution are reasonable.
- `constrained`: expected to fit with limited headroom.
- `remote-recommended`: accelerator fit failed and CPU fallback is impractical.
- `unsupported`: working set exceeds a safe share of memory.

Session creation refuses unsafe fits unless the user explicitly supplies `--force`. The policy is model-name independent. Context growth, KV cache, concurrency, and other processes can still change real memory use, so fit is a safety estimate rather than a guarantee.
