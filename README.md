# drover-libs

![Status: active](https://img.shields.io/badge/status-active-2ea44f)

> **Status, checked 18 Nov 2026: ACTIVE.** Shared Go module: `pkg/clock`, plus `pkg/telemetry` and `pkg/sessionevent` for M1. v0.1.0 prep merged 18 Nov 2026 (#10); not tagged yet.
>
> **Clean-clone check:** `go build ./...` and `go test -short ./...` pass. No version tag yet, so consumers use `replace` or a vendored copy.
>
> **Roadmap:** [ROADMAP.md](ROADMAP.md). Program tracking: Drover gap program (M0 to M4).

> **Language note (rationalization 2026-11-09):** GitHub's primary-language badge may show **Python** because `scripts/quality-gate.py` outweighs the Go sources by byte size. **Runtime surface is Go** — currently `pkg/clock` (`Clock`, `RealClock`, `MockClock`). Keep this tiny shared module; do not rewrite or retire it because of the language badge. Prefer publishing/pinning a real Go module version over sibling `replace` checkouts.

Shared Go libraries for the [Drover](https://github.com/cloud-shuttle) ecosystem.

## Packages

| Path | Purpose |
|------|---------|
| `pkg/clock` | `Clock` interface, `RealClock`, and `MockClock` for deterministic tests |
| `pkg/telemetry` | One OpenTelemetry bootstrap: OTLP export, W3C propagation, parent-based sampling, `drover.*` correlation IDs on every span, GenAI span helpers |
| `pkg/sessionevent` | Session event schema v1 (org ADR-003), wire-compatible with the copies in `drover` and `drover-code` |

See [CHANGELOG.md](CHANGELOG.md) for what each version adds and [ROADMAP.md](ROADMAP.md) for what is next.

## Usage

```go
import "github.com/cloud-shuttle/drover-libs/pkg/clock"

c := clock.RealClock{}
_ = c.Now()
```

Telemetry, once in `main`:

```go
import "github.com/cloud-shuttle/drover-libs/pkg/telemetry"

shutdown, err := telemetry.Setup(ctx, telemetry.ConfigFromEnv("drover-kelpies", version))
if err != nil {
	return err
}
defer shutdown(context.Background())

ctx = telemetry.WithIDs(ctx, telemetry.IDs{TenantID: tenant, SessionID: session})
ctx, span := telemetry.StartChat(ctx, "drover-gateway", model)
defer func() { telemetry.End(span, err) }()
```

Export turns on when `OTEL_EXPORTER_OTLP_ENDPOINT` (or `DROVER_OTEL_ENDPOINT`) is set. Propagation is always on, so a service with export off still passes `traceparent` through.

## Development

Consumers today: `drover-cloud` and `drover-warden` (`replace ../drover-libs`, so a sibling clone is required), and `drover-muster` and `drover-gateway` (vendored copies under `third_party/drover-libs`). There is no version tag yet; tagging v0.1.0 is the first M0 step so consumers can drop `replace`.

```bash
git clone git@github.com:cloud-shuttle/drover-libs.git
# sibling to drover-gateway:
#   ../drover-libs
```

Go **1.26.2**.
