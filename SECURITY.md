# Security Policy

## Reporting a Vulnerability

Please report vulnerabilities privately, **not** as a public GitHub issue.

- Preferred: open a **[private security advisory](https://github.com/barnes-c/ovs-exporter/security/advisories/new)**
  on GitHub.
- Alternative: email `github@barnes.biz` with `[ovs-exporter security]` in
  the subject line.

## Verifying Releases

Verify a container image:

```bash
cosign verify ghcr.io/barnes-c/ovs-exporter:<TAG> \
  --certificate-identity-regexp "^https://github.com/barnes-c/ovs-exporter/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Verify a downloaded binary tarball against the signed checksums:

```bash
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp "^https://github.com/barnes-c/ovs-exporter/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

If either verification fails for an artifact published from this repo,
treat it as a potential supply-chain incident and report it via the
process above.

Verify the checksums bundle:

```bash
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp "^https://github.com/barnes-c/ovs-exporter/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

Verify a container image:

```bash
cosign verify ghcr.io/barnes-c/ovs-exporter:<TAG> \
  --certificate-identity-regexp "^https://github.com/barnes-c/ovs-exporter/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Inspect the SBOM attestation bound to a container image:

```bash
cosign verify-attestation --type spdxjson \
  --certificate-identity-regexp "^https://github.com/barnes-c/ovs-exporter/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/barnes-c/ovs-exporter:<TAG>
```

Verify SLSA build provenance:

```bash
slsa-verifier verify-image \
  ghcr.io/barnes-c/ovs-exporter@sha256:<DIGEST> \
  --source-uri github.com/barnes-c/ovs-exporter \
  --source-tag <TAG>
```

If `cosign verify` fails for an artifact published from this repo, treat
it as a potential supply-chain incident and report it as a vulnerability.
