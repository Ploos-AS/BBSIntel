# M1.11 — v0.1.0 release preparation

Date: 2026-09-09

## Goal

Prepare BBSIntel for a reproducible first public release without creating the `v0.1.0` tag yet.

## Versioning

- `VERSION` declares `0.1.0`.
- `internal/buildinfo` carries version, commit, and build date.
- `bbsintel version`, `bbsintel --version`, and `bbsintel -version` print build metadata without opening the database.
- `GET /api/v1/version` exposes the same metadata through the HTTP API.
- release builds inject metadata with Go linker flags.

## OCI metadata

The Dockerfile accepts `VERSION`, `VCS_REF`, and `BUILD_DATE` build arguments and writes matching OCI labels. Release images embed the same values in the BBSIntel binary.

## Release workflow

`.github/workflows/release.yml` is tag-triggered for `v*` tags and first verifies that the tag exactly matches `v$(cat VERSION)`.

The release workflow then:

1. runs dependency cleanliness, gofmt, vet, and unit tests;
2. builds Linux amd64 and arm64 binaries;
3. embeds version/commit/build-date metadata;
4. produces SHA-256 files for both binaries;
5. publishes a multi-architecture image to `ghcr.io/ploos-as/bbsintel` with version and `latest` tags;
6. publishes the same image to `<DOCKERHUB_USERNAME>/bbsintel` when `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets are available;
7. creates the GitHub Release and attaches binaries/checksums.

## Release notes

`CHANGELOG.md` contains the v0.1.0 feature and qualification summary.

## Previous qualification evidence

M1.10 live qualification passed on 2026-09-09 with:

- 909 Telnet BBS Guide entries
- 356 Synchronet entries
- 1,026 deduplicated BBS identities
- 1,132 endpoints
- 95 multi-source BBS identities
- 20 bounded passive qualification probes
- successful API smoke checks

## Release gate

M1.11 is complete when CI is green on the final release-prep HEAD. The actual `v0.1.0` tag and release execution are deliberately a separate step so a release is never created from an unqualified intermediate commit.
