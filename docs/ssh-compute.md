# SSH compute

SSH targets store host, username, port, optional identity-file reference, and a user-owned remote root. Private key contents are never stored. OpenSSH uses batch mode, keepalives, and `StrictHostKeyChecking=yes`; SSH config and agent behavior remain available.

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute test gpu-1
backpack compute list
backpack run smollm2-135m --compute gpu-1
```

The target probes Linux CPU/RAM and NVIDIA/CUDA information. The local runtime manager first selects and verifies a Linux runtime variant, then stages its files under `~/.backpack/runtimes/<engine>/<version>/<variant>` and verifies every SHA-256 remotely. Model artifacts use the parallel `~/.backpack/models/<model>/<revision>/<package>` cache. `llama-server` binds remote loopback; the local API reaches it through SSH forwarding.

No global remote installation or root access is required. Transfers use SCP and are not resumable yet. Runtime cache detection currently checks the installed manifest/executable before reuse; a future worker can perform richer remote repair. No real SSH smoke test runs unless a host is supplied; normal tests use a mock transport.
