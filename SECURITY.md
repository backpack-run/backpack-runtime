# Security

Report vulnerabilities privately through GitHub Security Advisories for `backpack-run/backpack-runtime`; do not open a public issue with exploit details. The API and internal workers are loopback-only. Do not log credentials, SSH keys, tokens, request secrets, or uploaded audio. Verify package/runtime checksums and immutable revisions before execution.

Only the trusted runtime and Python-environment catalogs may select executable code or dependency inputs. Model manifests select a declared engine/version but cannot inject shell commands or worker scripts. Managed Python distributions, package caches, environments, and configuration are contained under `BACKPACK_HOME`; Backpack does not inspect or mutate global Python. Uploaded transcription files use private temporary storage and are removed after each request.

SSH retains strict `known_hosts` checking, binds remote services to loopback, verifies content-addressed input/model/runtime transfers remotely, and does not store private key contents. Treat model formats that can execute code or deserialize pickle data as higher risk; only catalog-pinned trusted publishers should be eligible for those adapters.
