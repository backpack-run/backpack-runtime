# Release provenance

Backpack Runtime publishes three related forms of release evidence.

1. A Git tag identifies the source revision used by the release workflow.
2. GoReleaser publishes platform archives and `checksums.txt` to the corresponding GitHub Release.
3. On tagged releases, GitHub Actions creates artifact attestations for every `.zip`, `.tar.gz`, and `checksums.txt` output using GitHub's OIDC-backed attestation service.

The release workflow pins third-party actions by full commit SHA. Attestation steps run only for a tag; manually dispatched snapshot builds remain CI artifacts and are not public releases.

## Verify a downloaded archive

First compare the archive with the published `checksums.txt` using `sha256sum`, `shasum -a 256`, or `Get-FileHash -Algorithm SHA256`. Then, with a recent GitHub CLI, verify its build provenance:

```sh
gh attestation verify backpack_0.1.0-alpha.1_linux_amd64.tar.gz \
  --repo backpack-run/backpack-runtime
```

```powershell
gh attestation verify .\backpack_0.1.0-alpha.1_windows_amd64.zip `
  --repo backpack-run/backpack-runtime
```

Verification should report `backpack-run/backpack-runtime` as the source repository and the GitHub Actions release workflow as the signer. Verify the tag and source commit shown by GitHub before trusting the result. Release attestations provide provenance; they do not make model packages, runtime bundles, or locally generated artifacts part of the same attestation.

## Trust boundary

Checksums detect corruption and unexpected archive substitution relative to `checksums.txt`, but an archive and its checksum are hosted together. Artifact attestations add independently verifiable GitHub Actions identity and workflow provenance. Users still trust:

- repository administrators and branch/tag controls;
- GitHub and GitHub Actions;
- the pinned actions and GoReleaser version in the workflow;
- TLS and the local verifier; and
- the source revision and build process referenced by the attestation.

Backpack's separately managed model and runtime catalogs retain their own pinned revisions and SHA-256 verification. Their provenance is documented with those catalogs rather than implied by the CLI archive attestation.

## Publishing policy

Release assets are immutable. If an archive or installer is wrong, publish a new semantic prerelease or patch release; never replace an asset under an existing tag. Alpha and beta tags must remain GitHub prereleases. A stable release may be selected only through the stable channel and must have no prerelease suffix.

The public `backpack.run` installer endpoints, once deployed, must serve reviewed static scripts traceable to this repository. See the hosting contract in [Install](install.md).
