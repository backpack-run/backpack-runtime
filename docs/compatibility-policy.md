# Compatibility policy

Backpack Runtime follows semantic versioning, including prerelease identifiers. The `0.x` alpha channel is intentionally unstable: breaking changes are allowed when documented in release notes. A beta release will narrow that freedom; stable compatibility is not promised before `1.0.0`.

## Interface status

| Interface | Alpha policy | Beta target |
|---|---|---|
| CLI commands and flags | May change; scripts should pin a Backpack version and prefer `--json` | Existing commands and flags remain compatible within a minor line, with removals announced first |
| Human-readable CLI output | Not a machine contract | Still not a machine contract |
| JSON CLI output | Versioned with the command; additive fields may appear | Existing fields retain meaning within a minor line |
| `/v1` OpenAI-compatible API | Best-effort supported subset; deviations are documented | Compatible supported fields remain stable; unsupported OpenAI fields may still be rejected |
| `/api/backpack/v1` | Versioned route, but alpha payloads may change with release notes | Additive evolution by default; incompatible payloads require a new route version |
| Model manifest | Only explicitly supported schema versions are accepted | Old supported schemas normalize forward; a new schema is opt-in, never guessed |
| Trusted runtime catalog | Internal signed-ready distribution contract; exact revisions and hashes are authoritative | Schema changes require an explicit reader migration |
| Python worker protocol | Private runtime boundary, version-negotiated by trusted definitions | A protocol version remains usable for the lifetime of its pinned runtime definition |
| `pkg/client` | Source API may change during alpha | Source compatibility is targeted within a minor line; Go modules remain pre-v1 |

The runtime rejects unknown future manifest schemas. This prevents a newer package contract from being partially interpreted by an older binary. Model repositories remain data-only: no compatibility policy permits manifests to inject shell commands, Python requirements, or arbitrary executable arguments.

## Deprecation and release notes

Alpha breaking changes must be called out in the corresponding GitHub release notes. Before beta, deprecated public API fields or CLI flags should normally survive at least one minor release unless retaining them would preserve a security vulnerability. Security fixes may break compatibility immediately and will be documented.
