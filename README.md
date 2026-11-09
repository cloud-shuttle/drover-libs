# drover-libs

> **Language note (rationalization 2026-11-09):** GitHub's primary-language badge may show **Python** because `scripts/quality-gate.py` outweighs the Go sources by byte size. **Runtime surface is Go** — currently `pkg/clock` (`Clock`, `RealClock`, `MockClock`). Keep this tiny shared module; do not rewrite or retire it because of the language badge. Prefer publishing/pinning a real Go module version over sibling `replace` checkouts.

Shared Go libraries for the [Drover](https://github.com/cloud-shuttle) ecosystem.

## Packages

| Path | Purpose |
|------|---------|
| `pkg/clock` | `Clock` interface, `RealClock`, and `MockClock` for deterministic tests |

## Usage

```go
import "github.com/cloud-shuttle/drover-libs/pkg/clock"

c := clock.RealClock{}
_ = c.Now()
```

## Development

`drover-gateway` consumes this module via `go.work` (sibling clone) or the copy vendored at `third_party/drover-libs` when a standalone checkout is used.

```bash
git clone git@github.com:cloud-shuttle/drover-libs.git
# sibling to drover-gateway:
#   ../drover-libs
```

Go **1.26.2**.
