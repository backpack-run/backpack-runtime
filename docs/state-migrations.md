# Persisted state and migrations

Backpack keeps user-owned state below `BACKPACK_HOME`. Model and runtime artifacts are immutable caches and can be re-created from their trusted revisions and checksums; configuration and generated outputs are user data and must not be silently discarded.

## Current inventory

| State | Location | Recovery rule |
|---|---|---|
| Daemon ownership | `state/runtime.json`, `state/startup.lock` | Reconcile PID and endpoint; stale ownership may be replaced |
| Sessions | `state/sessions.json` | Reconcile persisted metadata with live processes; never trust a PID alone |
| Jobs | `state/jobs.json` | Preserve terminal jobs; interrupted active jobs become failed/cancelled after reconciliation |
| Compute targets | `config/compute-targets.json` | Preserve user configuration; never store key contents or credentials |
| Installed model records | `manifests/<model>.json` plus immutable model directories | Verify revision and hashes; corrupt/missing cache can be downloaded again |
| Runtime records | versioned runtime directories and installed manifests | Verify trusted catalog version and per-file hashes; corrupt cache can be quarantined/reinstalled |
| Generated artifacts | `outputs/<job-id>/` | Preserve by default; paths exposed through the API remain job-confined |

## Versioning rules

Every persisted JSON document that must evolve will gain a top-level `schema_version`. Readers must distinguish three cases: migrate known older versions, read the current version, and reject unknown newer versions with an actionable error. Writes use a temporary file in the destination directory followed by atomic replacement and must propagate failures.

Migrations are ordered, deterministic, idempotent, and covered by fixtures for every supported source version. A migration writes a backup before modifying non-reconstructable configuration. Reconstructable cache metadata may instead be rebuilt after checksum verification. Migration code must never launch a model, install dependencies, contact a compute target, or execute manifest-provided content.

## Release policy

For alpha, release notes may instruct users to reset reconstructable daemon/session/cache metadata when no safe migration exists. Compute targets and generated outputs are not reset automatically. Before beta, persisted configuration, sessions/jobs, model pointers, and runtime metadata must either have versioned readers or be explicitly classified as disposable cache. Before stable, upgrade and rollback fixtures must cover every retained schema version.
