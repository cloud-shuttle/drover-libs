# Changelog

All notable changes to `github.com/cloud-shuttle/drover-libs`. This module follows [Semantic Versioning](https://semver.org/); while the major version is 0, a minor bump may change an API and a patch bump never does.

## Unreleased: v0.2.0 (proposed, M1 Trace spine)

### Added

- `pkg/telemetry`: one OpenTelemetry bootstrap for every Drover service. `ConfigFromEnv` + `Setup` install OTLP (gRPC or HTTP) trace and metric export, W3C TraceContext and Baggage propagation (also when export is off), a parent-based sampler so one run keeps one trace, and a span processor that stamps `drover.tenant.id`, `drover.session.id`, `drover.run.id`, `drover.turn.index` from the context. GenAI helpers (`StartChat`, `StartTool`, `StartAgent`, `RecordUsage`) use the attribute names drover-gateway already writes. `InjectMap` / `ExtractMap` / `Environ` / `ContextFromEnviron` carry trace context across job contracts and subprocesses.
- `pkg/sessionevent`: the shared session event schema v1 (org ADR-003), promoted from the copies in drover and drover-code with a byte-for-byte wire compatibility test. Adds optional `turn`, `trace_id` and `span_id` fields, `Validate`, `KnownType`, and `Encode` / `Decode` / `Marshal` for non-file writers.

## Unreleased: v0.1.0 (proposed)

First tagged release, so consumers can `go get github.com/cloud-shuttle/drover-libs@v0.1.0` instead of a sibling `replace ../drover-libs` or a vendored `third_party/drover-libs` copy.

### Added

- `pkg/clock`: `Clock` interface, `RealClock`, and `MockClock` (concurrency-safe, moved with `Add` and `SetTime`). The API is unchanged from the untagged code that drover-cloud, drover-warden, drover-muster and drover-gateway already use.
- Package documentation and the first tests for `pkg/clock`, including a race test for `MockClock`.
