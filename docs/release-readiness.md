# Release readiness

Every coding-agent-focused alpha must pass:

- clean build, tests, vet, and release snapshot
- Windows archive install, `--version`, `doctor`, and CLI help
- verified model/runtime download and at least one real GGUF inference path
- daemon restart, detached session, `ps`, stop, and no-orphan checks
- protocol translation tests for Chat Completions, Responses, and Messages
- capability refusal and launch-adapter configuration tests
- archive/checksum/install-script safety checks
- current model, agent, API, security, and license documentation

Platform and integration claims must name their evidence. Linux/macOS builds, SSH, desktop apps, and third-party agent binaries remain experimental until real environment qualification succeeds. A model that can generate text is not automatically agent-qualified.

Removed media commands, endpoints, runtime definitions, dependencies, and documentation must remain absent. Historical release notes may describe older alpha functionality but are not current product documentation.
