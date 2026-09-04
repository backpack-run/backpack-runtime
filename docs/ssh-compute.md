# SSH compute

SSH targets store host, username, port, optional identity-file reference, and a user-owned remote root. Private key contents are never stored. OpenSSH uses batch mode, keepalives, and `StrictHostKeyChecking=yes`; SSH config and agent behavior remain available.

```console
backpack compute add ssh gpu-1 --host gpu.example.org --user alice
backpack compute test gpu-1
backpack compute list
backpack run smollm2-135m --compute gpu-1
```

The target probes Linux CPU/RAM and NVIDIA/CUDA information. Before launch it creates `~/.backpack/models/<model>/<revision>/<package>`, compares remote SHA-256 values, skips valid artifacts, uploads missing content to `.part`, verifies it remotely, and atomically renames it. `llama-server` binds remote loopback; the local API reaches it through SSH forwarding.

Current limitation: automatic installation of a manifest-compatible remote llama.cpp build is intentionally not guessed. If `llama-server` is absent, an actionable error is returned. SCP transfer is not resumable. No real SSH smoke test runs unless a host is supplied; normal tests use a mock transport.
