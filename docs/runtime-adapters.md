# Runtime adapters

An adapter is selected from the normalized manifest `runtime.engine`. It validates requirements, prepares an engine, starts it through a `ComputeTarget`, checks health, advertises capabilities, and stops its session. Adapters must not select behavior from model IDs. Engine-specific dependencies belong in versioned runtime requirements or runtime-service manifests. Tests should use fake compute targets and processes; real smoke tests should use the smallest practical package.

