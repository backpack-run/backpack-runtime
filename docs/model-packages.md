# Model package runtime requirements

Model packages describe the model; runtime bundles describe executable environments. A package selects behavior through its manifest contract, never its repository name. Schema-v1 package runtime fields are normalized into `engine`, deterministic `version`, `environment`, and optional protocol version. New packages should write that normalized requirement explicitly.

`native-bundle` requirements are satisfied from the trusted shared runtime catalog. `isolated-python` requirements will be satisfied by pinned worker environments. A model package may still contain a model-specific runtime service descriptor where the worker itself is part of the validated model contract, but shared engines must not be duplicated into every model repository.

Model artifacts download to a sibling `.part` file. An interrupted transfer is retained and resumed only when the server returns `206 Partial Content` with a `Content-Range` beginning at the exact retained byte count. If the server ignores the range and returns a complete response, Backpack safely truncates and restarts the partial. The declared artifact size and SHA-256 are verified before the file is atomically installed; incomplete partials remain resumable, while a completed artifact with the wrong digest is discarded.
