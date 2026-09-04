# Managed runtime bundles

Backpack treats executable inference engines as versioned supply-chain artifacts. A model manifest declares a normalized requirement; the embedded runtime catalog maps it to one or more OS, architecture, and accelerator variants. The first catalog entry is the upstream llama.cpp `b10618` release, whose artifact digests are pinned from the project's GitHub artifact attestation.

Selection order is CUDA, Vulkan, then CPU on machines reporting CUDA; Vulkan then CPU on other Vulkan hosts; Metal then CPU on Apple Silicon; otherwise CPU. A missing variant is an error, not a download from an untrusted fallback. Large-model CPU policy warnings remain future work.

Downloads require HTTPS. Archives are SHA-256 verified before extraction. Absolute paths, directory traversal, symlinks, devices, excessive file counts, and unsupported formats are rejected. Extraction occurs in a sibling staging directory; a manifest records the source, license, executable, and every installed file digest before an atomic rename. A cross-process directory lock prevents competing CLI/service installs. A damaged installation is retained with an `.invalid-<timestamp>` suffix for diagnosis rather than overwritten in place.

The built-in catalog is intentionally deterministic rather than `latest`. `BACKPACK_RUNTIME_CATALOG` may point to a local catalog for development, but production distribution should update the reviewed embedded catalog. `BACKPACK_LLAMA_SERVER` and a PATH installation remain developer overrides.

Commands:

```console
backpack runtime list
backpack runtime show llama.cpp
backpack runtime install llama.cpp
backpack runtime verify llama.cpp
backpack runtime remove llama.cpp <version> <variant>
```

Installed layout:

```text
$BACKPACK_HOME/runtimes/<engine>/<version>/<variant>/
    manifest.json
    <upstream files>
```

Backpack currently consumes official upstream llama.cpp release binaries. If Backpack later publishes modified binaries, the publisher must build from a pinned source revision, retain licenses/notices, produce provenance attestations and SHA-256 digests, publish immutable archives, then update the catalog in review. Generated archives do not belong in this source repository.
