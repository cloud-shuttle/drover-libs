# drover-libs

![Status: stale](https://img.shields.io/badge/status-stale-d29922)

> **Status, checked 18 Nov 2026: STALE.** Tiny shared Go module, today only `pkg/clock`. Last code change 15 May 2026. Becomes the home of `pkg/telemetry` and `pkg/sessionevent` in M1.
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

Planned in M1 (not built yet): `pkg/telemetry` (one OpenTelemetry bootstrap with Drover attribute keys) and `pkg/sessionevent` (the session event schema now duplicated in `drover` and `drover-code`). See [ROADMAP.md](ROADMAP.md).

## Usage

```go
import "github.com/cloud-shuttle/drover-libs/pkg/clock"

c := clock.RealClock{}
_ = c.Now()
```

## Development

Consumers today: `drover-cloud` and `drover-warden` (`replace ../drover-libs`, so a sibling clone is required), and `drover-muster` and `drover-gateway` (vendored copies under `third_party/drover-libs`). There is no version tag yet; tagging v0.1.0 is the first M0 step so consumers can drop `replace`.

```bash
git clone git@github.com:cloud-shuttle/drover-libs.git
# sibling to drover-gateway:
#   ../drover-libs
```

Go **1.26.2**.
