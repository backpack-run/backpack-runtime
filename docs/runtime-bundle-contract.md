# Runtime bundle contract

Model packages declare a normalized engine requirement; the reviewed runtime catalog supplies reusable engine binaries for each platform/backend; Backpack resolves and installs them.

```yaml
runtime:
  engine: llama.cpp
  version: b10618
  environment: native-bundle
```

Legacy GGUF manifests normalize `runtime.provider` and `tested_revision` without consulting model IDs. Unknown revisions fail closed. Installed bundles follow `schemas/runtime-bundle-v1.schema.json` and record the engine, variant, executable, provenance, license, and per-file hashes.
