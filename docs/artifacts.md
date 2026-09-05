# Generated artifacts

Generated outputs live by default under:

```text
$BACKPACK_HOME/outputs/<job-id>/
```

Artifact metadata records an ID, media type, filename, byte size, format, creation time, image dimensions when available, and duration when supplied by a runtime. Internal absolute paths are deliberately excluded from API JSON.

Job and artifact identifiers are validated, and every resolved download path must remain inside the owning job directory. Traversal, absolute-path injection, and symlink escape are rejected. A future authenticated non-loopback service will need authorization in addition to this confinement; the current API remains loopback-only.

An explicit CLI output path is a client-side copy destination. It does not change the service's controlled artifact store or permit a remote request to choose an arbitrary server path.
