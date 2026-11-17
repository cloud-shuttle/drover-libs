# drover-libs roadmap

Last updated 18 Nov 2026. Status: **STALE**. Tiny shared Go module, today only `pkg/clock`. Last code change 15 May 2026. Becomes the home of `pkg/telemetry` and `pkg/sessionevent` in M1.

## The Drover gap program

In November 2026 Drover was compared with Vercel eve 1.0. Drover leads on isolation, the model gateway and governance, and trails on the layer people touch. The program closes five gaps across the repos: durable multi-turn sessions (drover-kelpies), a Slack-first channels layer (drover-cloud), unified OpenTelemetry tracing with a run viewer (drover-libs, Cloud console), a TypeScript client SDK (drover-cloud), and an extension bundle format (drover-muster, drover-registry). Four capabilities are deferred to Later: shared multi-person sessions, ambient channel listening, agents editing their own code, and session prewarm and migration.

Program tracking issue: in [cloud-shuttle/drover-cloud](https://github.com/cloud-shuttle/drover-cloud/issues?q=is%3Aissue+label%3Aroadmap+Tracking) (private). Sessions run on DBOS, not Temporal (ADR 0005 in drover-kelpies). Dates and day counts are estimates.

| Milestone | Due (estimate) |
|---|---|
| M0 Truth-up | 27 Nov 2026 |
| M1 Trace spine | 18 Dec 2026 |
| M2 Durable sessions | 5 Feb 2027 |
| M3 Channels and SDK | 5 Mar 2027 |
| M4 Extensions | 2 Apr 2027 |
| Later | after M4 |

## This repo in the program

### M1 Trace spine (due 18 Dec 2026)

- [pkg/telemetry: one OpenTelemetry bootstrap for every Drover service](https://github.com/cloud-shuttle/drover-libs/issues/6)
- [pkg/sessionevent: shared v1 session event schema](https://github.com/cloud-shuttle/drover-libs/issues/7)
- [Release v0.2.0 with pkg/telemetry and pkg/sessionevent](https://github.com/cloud-shuttle/drover-libs/issues/8)

Each item is a GitHub issue labelled `roadmap` with a `gap:*` label and a milestone: [roadmap issues in this repo](https://github.com/cloud-shuttle/drover-libs/issues?q=is%3Aissue+label%3Aroadmap).

## Proposed releases

- `v0.1.0` at M0 (pkg/clock as-is)
- `v0.2.0` at M1 (pkg/telemetry, pkg/sessionevent)

## Existing roadmap epics

These predate the gap program and stay open:

- [#4 Epic: Shared packages beyond clock](https://github.com/cloud-shuttle/drover-libs/issues/4)
- [#3 Epic: Publish a versioned Go module](https://github.com/cloud-shuttle/drover-libs/issues/3)

## How this file is kept current

Update this file in the same PR that closes a roadmap issue or moves a milestone. Status words used across Drover: ACTIVE (code changed in the last 60 days), STALE (no code change for 61 to 180 days), PROTOTYPE (experimental or scaffold), SUPERSEDED (replaced by another repo).
