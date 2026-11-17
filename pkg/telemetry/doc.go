// Package telemetry is the one OpenTelemetry bootstrap for Drover services.
//
// A service calls Setup once at start-up and defers the returned shutdown:
//
//	cfg := telemetry.ConfigFromEnv("drover-kelpies", version)
//	shutdown, err := telemetry.Setup(ctx, cfg)
//	if err != nil {
//		return err
//	}
//	defer shutdown(context.Background())
//
// Setup installs the global tracer and meter providers, exporting over OTLP
// to the collector named by the standard OTEL_EXPORTER_OTLP_* variables (or
// DROVER_OTEL_ENDPOINT, which drover already uses). It always installs the
// W3C TraceContext and Baggage propagators, even when export is disabled, so
// a service that does not export still passes traceparent through and the
// trace stays whole.
//
// The sampler is parent-based: a span with a sampled parent is always
// sampled, so one run keeps one trace across Cloud, Kelpies, Code, Gateway,
// Guard, Warden and Muster. Root spans are sampled at Config.SampleRatio.
//
// Every span started under a context carrying IDs (see WithIDs) gets the
// drover.tenant.id, drover.session.id, drover.run.id and drover.turn.index
// attributes stamped at start, so call sites do not repeat them. These IDs
// stay in-process and are never put in baggage, because baggage is sent on
// outbound requests, including to model providers.
//
// GenAI spans follow the OpenTelemetry GenAI semantic conventions, using the
// same attribute names drover-gateway's otel plugin already writes into the
// ClickHouse otel_traces table.
package telemetry
