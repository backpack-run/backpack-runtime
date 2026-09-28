# SSH compute

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute doctor gpu-1
backpack compute list
backpack run qwen3-coder-30b-a3b --compute gpu-1
```

Targets store connection metadata and an optional identity-file path, never private-key contents. OpenSSH uses batch mode, keepalives, and `StrictHostKeyChecking=yes`.

Backpack probes Linux CPU/RAM and NVIDIA/CUDA details, stages the locally verified llama.cpp bundle and model package in a user-owned remote cache, verifies hashes remotely, and atomically finalizes files. `llama-server` binds remote loopback and the local daemon connects through SSH forwarding. No root or global runtime installation is required.

`compute test` and `compute doctor` run connectivity, known-host, hardware, writable-root, transfer/checksum, and execution checks. Real-host qualification remains experimental unless a host is explicitly supplied; CI uses deterministic mock transports.
