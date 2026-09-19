# Backpack Cloud

Backpack Cloud is a separate optional managed product currently in private preview. Open-source Backpack Runtime requires no account for its public catalog, model installation, local execution, local API, SSH/user-owned compute, or local coding-agent integrations. Those paths do not contact Cloud and remain usable when `backpack.run` is unavailable.

The private-preview compatibility path can route hosted models through the same local loopback API. Availability is discovered from `GET https://api.backpack.run/v1/models`; it is not duplicated into the OSS catalog.

## Sign in

Interactive users authorize a device without giving the CLI a password:

```console
backpack login
backpack cloud status
backpack cloud models
```

The CLI creates an Ed25519 key locally, opens the verification page, and signs short-lived challenges after approval. The private key never leaves the device. Access tokens remain in memory and are renewed with another signed challenge. Windows protects the key with user-scoped DPAPI. Linux and macOS use a private `0600` file until native keychain integrations are added.

The command first explains that login is optional and Cloud-only. The Cloud backend remains the sole authority for private-preview admission and managed-compute entitlement. Runtime contains no identity allowlist and does not infer access from an email address or locally cached plan.

For non-interactive automation, set `BACKPACK_API_KEY` in the process environment. Do not put keys in command arguments, source control, logs, or shared shell profiles. The environment variable takes precedence over a saved device credential.

`backpack logout` deletes the local device credential. It cannot unset a parent shell's `BACKPACK_API_KEY`; remove that separately. Revoke the device in the Backpack account to invalidate it server-side.

## Cloud models and coding agents

The existing hosted API still reports transitional `:cloud` IDs. Always use an ID returned by `backpack cloud models`; Runtime does not invent or silently map one. At this release, the qualified available model is:

```text
qwen3-coder-30b-a3b-instruct:cloud
```

For example:

```console
backpack launch doctor codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch claude --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch claude-app --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch opencode --model qwen3-coder-30b-a3b-instruct:cloud
```

`qwen3-coder-next:cloud` is not currently advertised by the API and is not silently mapped to another model. Disabled or unknown IDs fail before an agent is launched and list the currently available IDs.

The local daemon forwards Cloud requests for OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages. It replaces the local per-daemon bearer token with the Cloud credential; the Cloud credential is never given to Codex, Claude Code, or OpenCode. Cloud launch is stateless, so `--keep-alive` and local/SSH compute selection do not apply.

If Windows reports that the stored credential cannot be opened or mentions DPAPI, the credential was encrypted for a different Windows user/machine state. Reinstalling the executable does not repair it. Run `backpack logout`, then `backpack login`, and verify with `backpack cloud status` and `backpack cloud models`.

An agent error such as `stream closed before response.completed` means the upstream worker ended a streaming request without a terminal protocol event. Current runtime builds convert that into an explicit protocol failure with the safe request ID when available. Retry only after `backpack cloud models` succeeds; if direct Cloud requests continue returning HTTP 5xx, the hosted GPU worker—not the local agent configuration—requires repair.

## Security boundary

The runtime remains loopback-only. Cloud proxy routes additionally require a random per-daemon bearer token stored in private runtime state and injected only into the launched child process. This protects against unauthenticated browser and local HTTP requests consuming Cloud quota. A malicious process already running as the same OS user remains within the same trust boundary and may be able to read user-owned process or state data.

`BACKPACK_CLOUD_URL` exists for development and private deployments. It accepts HTTPS endpoints, or loopback HTTP for tests; credentials are never sent to arbitrary cleartext hosts. Diagnostics report only whether authentication is configured and its source, never key material or access tokens.

Cloud is private alpha. Models, pricing, qualification, and availability may change independently of a runtime release. The live `/v1/models` response is authoritative.

## Model and target direction

The durable architecture keeps the concerns separate:

```text
model   = what runs
runtime = how it runs
target  = where it runs
launch  = what consumes it
```

Local and SSH targets require no Backpack account. A future `cloud` target will require Backpack authentication and current server-side entitlement. The current `:cloud` IDs remain only for private-preview compatibility until the Cloud catalog exposes a safe model-to-target contract; removing them in Runtime alone would require an unsafe hard-coded mapping. No new feature should depend on suffix-based identity.

An `account` CLI command is intentionally deferred. The current account profile endpoint accepts a browser identity token, whereas Runtime stores a device credential for inference. Runtime will not misuse one credential type as the other or display internal entitlement fields. A future device-safe account endpoint may expose only product language such as plan and Cloud availability.
