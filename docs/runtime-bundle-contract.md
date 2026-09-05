# Runtime bundle contract

The scalable contract is hybrid:

- `backpack-model-packager` publishes model requirements and immutable model artifacts.
- A separate Backpack runtime catalog publishes reusable engine/version/platform references and verified artifact digests.
- `backpack-runtime` resolves requirements, installs bundles, and passes executables to adapters/compute targets.

New model manifests should declare, conceptually:

```yaml
runtime:
  engine: llama.cpp
  version: b10618
  environment: native-bundle
```

Legacy schema-v1 GGUF packages normalize `runtime.provider` plus `tested_revision` without consulting the model ID. The catalog may explicitly map a tested source revision to a packaged runtime version. An unknown version/revision is rejected; Backpack never silently substitutes `latest`.

The installed bundle uses [`runtime-bundle-v1.schema.json`](../schemas/runtime-bundle-v1.schema.json). It records schema version, engine version, variant, OS, architecture, accelerator backend, relative executable, source/provenance, license, and SHA-256 for each installed file. The catalog—not each model repository—owns download URLs and archive digests, preventing binary duplication across model packages.

The packager's current `runtime.yaml`/`runtime_services` contract remains useful as verified publication metadata. Backpack consumes Whisper as a native job and qwen-asr through its common isolated-Python worker protocol; neither route depends on a model-name conditional. A future backward-compatible packager revision should emit the normalized top-level requirement directly and reference shared native bundles where an engine artifact is reusable.
