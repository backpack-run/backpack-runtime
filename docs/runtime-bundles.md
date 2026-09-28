# Managed runtime bundles

Backpack's trusted runtime catalog currently distributes pinned llama.cpp variants for Windows amd64, Linux amd64, and macOS arm64. Each entry records source revision, license, platform/backend, HTTPS artifacts, and SHA-256 digests.

Selection prefers CUDA, Vulkan, or Metal when a compatible variant is available, then CPU. Missing variants fail closed. Archives are verified and safely extracted into a sibling staging directory before atomic installation under:

```text
$BACKPACK_HOME/runtimes/<engine>/<version>/<variant>/
```

Use `backpack runtime list`, `show`, `install`, `verify`, and `remove` for inspection and repair. `BACKPACK_LLAMA_SERVER` remains a development override. Backpack never substitutes an unreviewed `latest` binary.
