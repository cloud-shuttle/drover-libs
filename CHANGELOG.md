# Changelog

All notable changes to `github.com/cloud-shuttle/drover-libs`. This module follows [Semantic Versioning](https://semver.org/); while the major version is 0, a minor bump may change an API and a patch bump never does.

## Unreleased: v0.1.0 (proposed)

First tagged release, so consumers can `go get github.com/cloud-shuttle/drover-libs@v0.1.0` instead of a sibling `replace ../drover-libs` or a vendored `third_party/drover-libs` copy.

### Added

- `pkg/clock`: `Clock` interface, `RealClock`, and `MockClock` (concurrency-safe, moved with `Add` and `SetTime`). The API is unchanged from the untagged code that drover-cloud, drover-warden, drover-muster and drover-gateway already use.
- Package documentation and the first tests for `pkg/clock`, including a race test for `MockClock`.

### Next

- v0.2.0 (M1 Trace spine): `pkg/telemetry` (OpenTelemetry bootstrap) and `pkg/sessionevent` (shared session event schema v1).
