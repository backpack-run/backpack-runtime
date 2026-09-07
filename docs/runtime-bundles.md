# Managed runtime bundles

Backpack treats executable inference engines as versioned supply-chain artifacts. A model manifest declares a normalized requirement; the embedded runtime catalog maps it to one or more OS, architecture, and accelerator variants. Current entries cover llama.cpp `b10618`, the pinned whisper.cpp revision validated by the packager, and uv 0.11.15 as the managed-Python bootstrap. Every archive digest is pinned.

Selection order is CUDA, Vulkan, then CPU on machines reporting CUDA; Vulkan then CPU on other Vulkan hosts; Metal then CPU on Apple Silicon; otherwise CPU. A missing variant is an error, not a download from an untrusted fallback. The model-fit policy separately refuses clearly impractical CPU fallback unless explicitly forced.

Downloads require HTTPS. Archives are SHA-256 verified before extraction. Absolute paths, directory traversal, hard links, devices, excessive file counts, and unsupported formats are rejected. Tar symlinks are accepted only when relative, lexically confined to the extraction root, and non-dangling after the complete link chain is created. Regular files are extracted before links so a link cannot redirect a later write. Extraction occurs in a sibling staging directory; a manifest records the source, license, executable, and every installed file digest before an atomic rename. A cross-process directory lock prevents competing CLI/service installs. A damaged installation is retained with an `.invalid-<timestamp>` suffix for diagnosis rather than overwritten in place.

The built-in catalog is intentionally deterministic rather than `latest`. `BACKPACK_RUNTIME_CATALOG` may point to a local catalog for development, but production distribution should update the reviewed embedded catalog. `BACKPACK_LLAMA_SERVER` and a PATH installation remain developer overrides.

Commands:

```console
backpack runtime list
backpack runtime show whisper.cpp
backpack runtime install whisper.cpp
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

Runtime bundles and the `backpack` CLI release are separate signed-ready artifacts. End users install one versioned CLI archive; the trusted runtime catalog then selects and verifies engine bundles as needed. Current execution validation is Windows x64. Linux amd64 and macOS arm64 catalog variants are published only after their exact upstream artifacts and execution paths are verified; build portability alone is not a support claim.
