# Compute targets

A compute target answers where an already-qualified coding model runs. The agent and protocol layers do not depend on that choice.

- `local`: hardware inspection plus owned process launch.
- `ssh`: strict known-host verification, remote hardware probing, verified cache synchronization, remote loopback launch, and local forwarding.
- Backpack Cloud: optional access-controlled compatibility path; it must never become a prerequisite for local or SSH operation.

All targets receive the same model/runtime contract. Actual availability depends on a trusted runtime variant and model-fit result. Large transfers prefer resumable rsync and fall back to non-resumable SCP with checksum validation and atomic finalization.
