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

Every persisted JSON document that must evolve uses a top-level `schema_version`. Readers distinguish three cases: migrate known older versions, read the current version, and reject unknown newer versions with an actionable error. Writes use a temporary file in the destination directory followed by atomic replacement and propagate failures where the owning API exposes them.

Migrations are ordered, deterministic, idempotent, and covered by fixtures for every supported source version. A migration writes a backup before modifying non-reconstructable configuration. Reconstructable cache metadata may instead be rebuilt after checksum verification. Migration code must never launch a model, install dependencies, contact a compute target, or execute manifest-provided content.

The current persisted-state schema version is `1`. Versioned list documents use a named envelope rather than a bare array:

```json
{
  "schema_version": 1,
  "targets": []
}
```

The corresponding collection keys are `targets`, `sessions`, and `jobs`. Installed-model pointer records retain their existing `repository`, `revision`, and `package` fields and add a top-level `schema_version`.

## Upgrade behavior from v0.1 alpha

The first alpha wrote compute targets, sessions, and jobs as bare arrays and installed-model pointers as unversioned objects. Readers recognize only those exact legacy shapes. On first read Backpack:

1. decodes the complete legacy document before making any change;
2. writes the original bytes to `<state-file>.v0.bak` with user-only permissions;
3. writes schema version 1 through a temporary file in the destination directory;
4. atomically replaces the active document; and
5. retains the backup for manual rollback or inspection.

The migration does not change collection entries or installed-model pointer fields. Session and job reconciliation then applies the existing restart rules to active records. Re-reading a version 1 document is idempotent and does not create another backup.

If the backup path already exists with different contents, Backpack refuses migration instead of overwriting evidence from an earlier upgrade. If a document declares a schema newer than this binary understands, reads and state-changing operations fail with an `upgrade Backpack` error and the file is left byte-for-byte unchanged.

## Implemented coverage

| Document | Current format | v0 migration | Future-version behavior |
|---|---|---|---|
| `config/compute-targets.json` | `{schema_version, targets}` | bare array to v1 with backup | list/add/remove refuse |
| `state/sessions.json` | `{schema_version, sessions}` | bare array to v1 with backup | session reads and mutations refuse; `StateError` exposes startup failure |
| `state/jobs.json` | `{schema_version, jobs}` | bare array to v1 with backup | job reads and mutations refuse; `StateError` exposes startup failure |
| `manifests/<model>.json` | versioned installed-model pointer | unversioned object to v1 with backup | installed-model resolution refuses |
| runtime `manifest.json` | version 1 (already present in alpha) | none required | runtime load/list refuses a newer version |
| Python `environment.json` | version 3 (already present in alpha) | reconstructable cache; incompatible metadata is rebuilt | a newer/invalid environment is not reused |

`state/runtime.json` is short-lived daemon ownership metadata rather than durable user configuration. New writes include schema version 1, legacy unversioned ownership is accepted without rewriting, and future versions are refused. It remains reconciled through endpoint health and PID ownership rather than migrated or trusted as durable state. Generated artifacts are files plus metadata embedded in versioned job entries; output bytes are never rewritten by a state migration.

The shared migration implementation lives in `internal/statemigrate`. It operates only on caller-supplied bytes and paths and has no network, process-launch, model, runtime, or compute dependencies.

## Release policy

For alpha, release notes may instruct users to reset reconstructable daemon/session/cache metadata when no safe migration exists. Compute targets and generated outputs are not reset automatically. Before beta, persisted configuration, sessions/jobs, model pointers, and runtime metadata must either have versioned readers or be explicitly classified as disposable cache. Before stable, upgrade and rollback fixtures must cover every retained schema version.
