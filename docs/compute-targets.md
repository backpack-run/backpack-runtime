# Compute targets

A compute target answers where work runs; it does not know model families. Local execution implements hardware inspection and managed process launch. SSH implements saved configuration, strict known-host verification, hardware probing, checksum-aware model/runtime/input synchronization, remote loopback launch, and local forwarding. Native job commands do not open unnecessary tunnels. Managed Backpack Compute remains an explicit unimplemented boundary.

The same adapter receives either target. Actual availability still depends on a trusted runtime variant for the target platform. Today llama.cpp has local and SSH-capable Linux bundles; Whisper is validated only on local Windows x64 CPU. Isolated-Python workers return an explicit unavailable error for SSH until pinned remote uv/Python/environment definitions are published and validated.
