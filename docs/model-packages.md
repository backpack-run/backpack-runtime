# Coding-model package requirements

Packages describe immutable model data; runtime bundles describe executable engines. Behavior is selected by manifest contract, never repository name. A coding package declares artifact identity, hashes, size, runtime requirement, context/capability metadata, and fit estimates.

Split GGUF packages must declare the complete contiguous shard set and use shard one as the entrypoint. Generic non-executable auxiliary data remains representable, but multimodal projector artifacts are rejected because vision is outside the current coding-text scope. A manifest cannot provide commands, arguments, dependency installers, environment variables, or worker code.

Downloads resume only after an exact HTTP range handshake. Declared size and SHA-256 are verified before atomic installation.
