# Managed Python runtime

Backpack uses uv as a pinned, verified bootstrap for Python-backed inference engines. It never installs packages into system Python, changes global `PATH`, or shares an uncontrolled environment between engine versions.

```text
runtimes/
  uv/0.11.15/<variant>/
  python/environments/
    qwen-asr/0.0.6/windows-amd64-cpu/
    kokoro/0.9.4/windows-amd64-cpu/
```

Environment selection is driven by the normalized manifest handshake: engine, exact runtime version, and `isolated-python` environment. Definitions select a pinned Python version, versioned uv hash lock, worker asset, platform, backend, and explicit package indexes. They do not inspect the model ID. `UV_PYTHON_INSTALL_DIR`, `UV_CACHE_DIR`, and `UV_NO_CONFIG` keep the interpreter, cache, and configuration inside Backpack's controlled boundary. A partially created environment is never treated as installed; Backpack validates its environment manifest and asset digests before cache reuse.

Environment schema v3 records the dependency-lock version, lock digest, index URLs, platform, worker digest, and every resolved distribution hash. Installation uses uv `--require-hashes`; hash checking is all-or-nothing. PyPI and the PyTorch CPU index are both explicit. Because the PyTorch index mirrors common names, exact pins and trusted hashes guard the required multi-index `unsafe-best-match` resolution. Any lock, source, platform, or worker change invalidates reuse.

The current trusted catalog supports Windows amd64 CPU for qwen-asr 0.0.6 and Kokoro 0.9.4. Global Python is neither detected nor used. Linux, macOS, GPU variants, and SSH Python bootstrapping remain unavailable until corresponding pinned definitions and real validation exist.
