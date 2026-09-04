# Runtime adapters

An adapter is selected from the normalized manifest `runtime.engine`. It validates requirements, asks the runtime manager to satisfy native/isolated dependencies, starts through a `ComputeTarget`, checks health, advertises capabilities, and stops its session. Adapters must not select behavior from model IDs or contain download URLs. Engine dependencies belong in versioned runtime requirements; reusable executable artifacts belong in the runtime catalog. Tests use fake targets/processes, while real smoke tests use the smallest practical package.
