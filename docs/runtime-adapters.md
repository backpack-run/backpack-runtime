# Runtime adapters

Runtime adapters translate a normalized model requirement into an engine process on a `ComputeTarget`. They own engine preparation, launch, health, capabilities, and stop behavior; they do not contain agent protocol types, model-name conditionals, or download URLs.

The shipped adapter is `llama.cpp`, providing persistent text-generation sessions for coding models. The registry remains extensible for a future trusted high-performance server backend. Adding an adapter requires deterministic runtime packaging, lifecycle tests, protocol-neutral inference behavior, and real execution qualification.
