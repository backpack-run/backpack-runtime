# Security

Report vulnerabilities privately through GitHub Security Advisories for `backpack-run/backpack-runtime`. Do not open a public issue containing exploit details.

Backpack treats model manifests as semi-trusted data. Only the reviewed model and runtime catalogs may select executable engines or downloads. Artifacts are pinned and SHA-256 verified; manifests cannot inject commands, arguments, dependencies, worker code, or environment variables.

The local API and inference engines bind to loopback. Launch credentials are random and scoped to the daemon; Cloud credentials remain inside Backpack and must not appear in logs or child environments. SSH uses strict known-host verification and never stores private-key contents.

Do not log credentials, tokens, SSH endpoints/keys, prompts, conversations, private paths, or tool results. `backpack doctor --json` is the preferred sanitized diagnostic output.

Release installers use HTTPS, verify GitHub Release checksums, reject unsafe archive members, and install without elevation by default. Protecting repository, workflow, and release credentials remains essential because archives and checksum files share the same release trust domain.
