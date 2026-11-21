# Changelog

All notable changes to `github.com/cloud-shuttle/drover-libs`. This module follows [Semantic Versioning](https://semver.org/); while the major version is 0, a minor bump may change an API and a patch bump never does.

## Unreleased

### Added

- `pkg/dbosboot`: one DBOS bootstrap for the three DBOS runtime owners (drover-cloud, drover-kelpies, the `drover` CLI). `PoolConfig` / `OpenSystemPool` / `Config` build a `pgpool.Session` system pool (always direct 5432 on PlanetScale, so DBOS's LISTEN/NOTIFY and migrations never hit PgBouncer), capped at 3 by default (`pool_max_conns` in the URL wins) or exactly at `MaxConns` (floor 2), with optional `Schema`, `ApplicationName`, lifetimes, connect timeout and `EnsureDatabase`. Returns a `dbos.Config` with `SystemDBPool` set. Adds the `github.com/dbos-inc/dbos-transact-golang` v0.15.0 dependency (only for importers of this package). Workflow registration, Launch and Shutdown stay in each service.

### Fixed

- `pkg/pgpool`: PlanetScale detection now also matches regional Postgres endpoints (`*.pg.psdb.cloud`, for example `aws-ap-southeast-2-1.pg.psdb.cloud`). Before, only `*.horizon.psdb.cloud` matched, so `Pooled` pools on a regional host stayed on direct port 5432 instead of PgBouncer on 6432, using up `max_connections` (SQLSTATE 53300). MySQL hosts (`*.connect.psdb.cloud`) still never match.

### Added

- `pkg/pgpool`: `PlanetScaleRegionalHostSuffix`, `PlanetScaleHostSuffixes`, and the `DROVER_PG_HOST_SUFFIXES` environment variable (comma-separated extra host suffixes).

## Unreleased: v0.2.0 (proposed, M1 Trace spine)

### Added

- `pkg/pgpool`: one pgx pool constructor (`New`, `Config`) where each pool is `Pooled` or `Session`. On a PlanetScale host (`*.horizon.psdb.cloud`), `Pooled` moves port 5432 to 6432 (PlanetScale's local PgBouncer, same credentials) and `Session` moves 6432 back to 5432. Every pool gets an explicit `MaxConns` (Pooled 3, Session 2, unless `WithMaxConns` or `pool_max_conns` says otherwise). Pooled pools on 6432 default to `QueryExecModeCacheDescribe`. `DROVER_PG_POOLER=auto|off|force` overrides detection. Adds the `github.com/jackc/pgx/v5` dependency. No service uses it yet.
- `pkg/telemetry`: one OpenTelemetry bootstrap for every Drover service. `ConfigFromEnv` + `Setup` install OTLP (gRPC or HTTP) trace and metric export, W3C TraceContext and Baggage propagation (also when export is off), a parent-based sampler so one run keeps one trace, and a span processor that stamps `drover.tenant.id`, `drover.session.id`, `drover.run.id`, `drover.turn.index` from the context. GenAI helpers (`StartChat`, `StartTool`, `StartAgent`, `RecordUsage`) use the attribute names drover-gateway already writes. `InjectMap` / `ExtractMap` / `Environ` / `ContextFromEnviron` carry trace context across job contracts and subprocesses.
- `pkg/sessionevent`: the shared session event schema v1 (org ADR-003), promoted from the copies in drover and drover-code with a byte-for-byte wire compatibility test. Adds optional `turn`, `trace_id` and `span_id` fields, `Validate`, `KnownType`, and `Encode` / `Decode` / `Marshal` for non-file writers.

## v0.1.0 (18 Nov 2026)

First tagged release, so consumers can `go get github.com/cloud-shuttle/drover-libs@v0.1.0` instead of a sibling `replace ../drover-libs` or a vendored `third_party/drover-libs` copy.

### Added

- `pkg/clock`: `Clock` interface, `RealClock`, and `MockClock` (concurrency-safe, moved with `Add` and `SetTime`). The API is unchanged from the untagged code that drover-cloud, drover-warden, drover-muster and drover-gateway already use.
- Package documentation and the first tests for `pkg/clock`, including a race test for `MockClock`.
