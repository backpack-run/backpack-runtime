# Third-party notices

Backpack Runtime source is Apache-2.0. Managed runtime bundles and model packages retain their own licenses.

The built-in runtime catalog currently references:

- [llama.cpp](https://github.com/ggml-org/llama.cpp), licensed under MIT. Backpack installs its pinned upstream license alongside the runtime.
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp), licensed under MIT. The trusted Backpack worker bundle includes the upstream license, an Apache-2.0 license for the Backpack protocol worker, and a notice identifying both components.
- [uv](https://github.com/astral-sh/uv), dual-licensed under Apache-2.0 or MIT. Backpack installs both pinned upstream license texts alongside the managed bootstrap executable.

Backpack records the upstream project, immutable release revision, license identifier, archive SHA-256, and installed file hashes. Where an upstream attestation is available, the catalog records it too.

This notice does not replace the license files shipped with a runtime or model.
