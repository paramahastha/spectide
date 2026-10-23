# Local release artifact preparation

SpecTide can prepare reproducible binary archives for the initial macOS and Linux target set. This is a preparation workflow only: it does not create a Git tag, GitHub Release, package-manager entry, or public download.

## Requirements and targets

- Go 1.27.2 (the version pinned by `go.mod`)
- Node.js 26 and npm, used to rebuild the embedded dashboard
- macOS or Linux host with `bash`, `tar`, and `python3`

The build matrix is `darwin/arm64`, `darwin/amd64`, `linux/arm64`, and `linux/amd64`, with CGO disabled. Windows and OS-specific installers are not included. Cross-target binaries are compile-checked on the Linux workflow host; only the host-compatible archive receives a runtime smoke check.

## Prepare and inspect artifacts

From the repository root, install dashboard dependencies and prepare artifacts:

```bash
cd web && npm ci && cd ..
make release-prep VERSION=1.0.0-rc.1
```

`make release-prep` rebuilds `internal/webassets/static` first, then creates `dist/release/` containing four `.tar.gz` archives, `release-metadata.json`, and `SHA256SUMS`. Archives use sorted paths and normalized timestamps/owners for repeatable packaging. The metadata records the version, source commit, whether the working tree was clean, Go and Node toolchain versions, embedded dashboard status, and target matrix. Each archive contains the versioned `spectide` binary, `VERSION`, `README.md`, and `LICENSE`.

The host-compatible archive is checked by running `spectide version`, `spectide doctor`, and the smoke script. The smoke test starts the server and verifies `/healthz`, the served dashboard HTML, and its embedded JavaScript entry, then completes the offline simulated demo. Other architectures are cross-compiled but cannot be runtime-verified on a different host. Verify package hashes with `sha256sum -c SHA256SUMS` (Linux) or `LC_ALL=C shasum -a 256 -c SHA256SUMS` (macOS; the locale override also works when Perl cannot load the host's default locale).

The manually triggered [Prepare release artifacts workflow](../.github/workflows/prepare-release.yml) runs this preparation on Linux and retains the workflow artifact for 14 days. It does not publish to GitHub Releases. Owner authorization and a separate release review are required before any public release, package upload, or publication.

## Readiness limits

Cross-build success and the offline smoke test do not establish authenticated provider acceptance, browser-rendered accessibility acceptance, or a clean official Spec-Kit convergence result. Those gates remain listed in the [acceptance ledger](acceptance-ledger.md) and [final verification](final-verification.md). The current product version and release scope still require project-owner approval before publication.
