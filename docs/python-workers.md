# Isolated Python workers

Python-backed adapters use Backpack-owned, versioned environments rather than global Python or pip state. Backpack installs the pinned uv 0.11.15 binary, asks uv for a managed Python distribution, and builds each environment under `runtimes/python/environments/<engine>/<version>/<variant>`. Successful environments contain an `environment.json` record with the Python version, protocol version, and SHA-256 identities of the dependency input and worker. Creation happens in a sibling partial directory before atomic publication.

Protocol v1 is private loopback HTTP:

- `GET /v1/health`
- `GET /v1/metadata`
- `POST /v1/load`
- `POST /v1/infer`
- `POST /v1/unload`
- `POST /v1/shutdown`

Workers return JSON errors at the service boundary. The Go daemon owns worker startup, readiness timeout, model loading, inference routing, shutdown, log capture, and child-process cleanup. Worker scripts are trusted runtime assets embedded in the Backpack executable; model packages cannot provide arbitrary executable commands.

Qwen3-ASR 0.6B uses Python 3.11 and the packager-published exact dependency set headed by `qwen-asr==0.0.6`. Kokoro uses Python 3.12 and a fully pinned dependency set headed by `kokoro==0.9.4`, including a SHA-256-pinned spaCy English model wheel. Both passed clean-home inference, cache/session reuse, and stop/no-orphan validation on Windows x64 CPU. Other platforms and SSH fail with an explicit compatibility error rather than falling back to global Python.
