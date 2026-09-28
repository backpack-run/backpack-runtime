# Persisted state and migrations

Backpack stores daemon ownership, sessions, compute targets, installed-model pointers, and runtime manifests beneath `BACKPACK_HOME`. Model/runtime artifacts are immutable reconstructable caches; compute configuration is user-owned and must not be silently discarded.

Durable JSON uses a top-level `schema_version`. Readers migrate known legacy shapes, read the current version, and reject unknown newer versions without modifying them. Migration is deterministic and atomic, and non-reconstructable configuration receives a backup before replacement.

Current versioned collections are `targets` and `sessions`. The former alpha `jobs` document is no longer part of the product; existing files are left untouched but ignored so user data is not destructively removed. Installed model pointers retain repository, revision, and package fields. Daemon ownership is short-lived and reconciled using live health rather than PID alone.

Before stable, every retained state schema must have upgrade and rollback fixtures. Alpha release notes may classify reconstructable cache metadata as resettable, but never silently reset compute targets or credentials.
