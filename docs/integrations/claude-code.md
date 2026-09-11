# Claude Code integration

`backpack launch claude` detects the real `claude` executable on `PATH`, ensures a Backpack model session, and configures only the child process.

The child receives a loopback `ANTHROPIC_BASE_URL`, a placeholder `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=backpack-runtime`, and an isolated `CLAUDE_CONFIG_DIR`. `ANTHROPIC_API_KEY` is removed. The selected Backpack model is mapped consistently to the default, Opus, Sonnet, Haiku, Fable, and subagent model variables so a launched session cannot silently select an Anthropic model tier.

Experimental betas, adaptive thinking, tool search, telemetry, error reporting, and nonessential traffic are disabled for the initial compatibility profile. Existing shell environment conflicts are overridden only in the child; normal `~/.claude` settings, plugins, and history are not edited or deleted.

The Messages parser, streaming text, `tool_use`, and `tool_result` paths have deterministic conformance tests. Claude Code was not installed on the qualification host, and Anthropic does not officially support using non-Claude models behind Claude Code gateways. This integration is therefore experimental external compatibility, not a claim of Anthropic support.
