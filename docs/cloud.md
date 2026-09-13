# Backpack Cloud

Backpack Runtime can route supported `:cloud` models through the hosted Backpack API while keeping the CLI and coding-agent integrations on the same local loopback API. Cloud availability is discovered from `GET https://api.backpack.run/v1/models`; it is not duplicated in the local catalog.

## Sign in

Interactive users authorize a device without giving the CLI a password:

```console
backpack login
backpack cloud status
backpack cloud models
```

The CLI creates an Ed25519 key locally, opens the verification page, and signs short-lived challenges after approval. The private key never leaves the device. Access tokens remain in memory and are renewed with another signed challenge. Windows protects the key with user-scoped DPAPI. Linux and macOS use a private `0600` file until native keychain integrations are added.

For non-interactive automation, set `BACKPACK_API_KEY` in the process environment. Do not put keys in command arguments, source control, logs, or shared shell profiles. The environment variable takes precedence over a saved device credential.

`backpack logout` deletes the local device credential. It cannot unset a parent shell's `BACKPACK_API_KEY`; remove that separately. Revoke the device in the Backpack account to invalidate it server-side.

## Cloud models and coding agents

Always use an ID reported by `backpack cloud models`. At this release, the qualified available model is:

```text
qwen3-coder-30b-a3b-instruct:cloud
```

For example:

```console
backpack launch doctor codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch codex --model qwen3-coder-30b-a3b-instruct:cloud
backpack launch claude --model qwen3-coder-30b-a3b-instruct:cloud
```

`qwen3-coder-next:cloud` is not currently advertised by the API and is not silently mapped to another model. Disabled or unknown IDs fail before an agent is launched and list the currently available IDs.

The local daemon forwards Cloud requests for OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages. It replaces the local per-daemon bearer token with the Cloud credential; the Cloud credential is never given to Codex, Claude Code, or OpenCode. Cloud launch is stateless, so `--keep-alive` and local/SSH compute selection do not apply.

## Security boundary

The runtime remains loopback-only. Cloud proxy routes additionally require a random per-daemon bearer token stored in private runtime state and injected only into the launched child process. This protects against unauthenticated browser and local HTTP requests consuming Cloud quota. A malicious process already running as the same OS user remains within the same trust boundary and may be able to read user-owned process or state data.

`BACKPACK_CLOUD_URL` exists for development and private deployments. It accepts HTTPS endpoints, or loopback HTTP for tests; credentials are never sent to arbitrary cleartext hosts. Diagnostics report only whether authentication is configured and its source, never key material or access tokens.

Cloud is private alpha. Models, pricing, qualification, and availability may change independently of a runtime release. The live `/v1/models` response is authoritative.
