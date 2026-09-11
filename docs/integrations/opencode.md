# OpenCode integration

`backpack launch opencode` detects the real `opencode` executable on `PATH`, ensures a Backpack model session, and starts OpenCode against Backpack's loopback OpenAI-compatible API.

Backpack supplies a child-only `OPENCODE_CONFIG_CONTENT` document containing an `@ai-sdk/openai-compatible` provider and selects `backpack/<model>`. It also uses a Backpack-owned `OPENCODE_CONFIG_DIR`, disables provider-model fetching, automatic updates, default plugins, Claude Code configuration import, and automatic sharing for the launched process. Normal user configuration is not modified.

The invocation uses OpenCode's `--pure` mode and rejects passthrough `--model` arguments that could replace Backpack's provider routing. It does not weaken OpenCode permissions or tool controls. Workspace access, tool execution, and user approvals remain OpenCode responsibilities.

The provider builder and configuration isolation have deterministic tests. OpenCode was not installed on the qualification host, and no Backpack coding model has completed an end-to-end OpenCode file-edit/test qualification, so this integration remains experimental.

See OpenCode's official [provider](https://opencode.ai/docs/providers), [configuration](https://opencode.ai/docs/config/), and [CLI](https://opencode.ai/docs/cli/) documentation for the upstream behavior Backpack integrates with.
