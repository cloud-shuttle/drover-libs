package telemetry

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Propagator is the W3C TraceContext + Baggage propagator Setup installs.
func Propagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// InjectMap returns the trace context of ctx as a string map
// ({"traceparent": ..., "tracestate": ...}). Use it to carry a trace across a
// boundary that is not HTTP: the Cloud to worker job contract, a queue
// message, a DBOS workflow input.
func InjectMap(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}

// ExtractMap returns ctx with the remote span context found in m, so spans
// started from it continue the caller's trace. A nil or empty map returns
// ctx unchanged.
func ExtractMap(ctx context.Context, m map[string]string) context.Context {
	if len(m) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(m))
}

// Environment variable names for passing trace context to a child process,
// as in the OpenTelemetry environment carrier specification.
const (
	EnvTraceparent = "TRACEPARENT"
	EnvTracestate  = "TRACESTATE"
	EnvBaggage     = "BAGGAGE"
)

// Environ returns "KEY=value" entries carrying the trace context of ctx, to
// append to exec.Cmd.Env when starting an agent or tool subprocess.
func Environ(ctx context.Context) []string {
	var env []string
	for k, v := range InjectMap(ctx) {
		env = append(env, strings.ToUpper(k)+"="+v)
	}
	return env
}

// ContextFromEnviron continues the trace a parent process passed in
// TRACEPARENT / TRACESTATE / BAGGAGE. Call it at the top of main.
func ContextFromEnviron(ctx context.Context) context.Context {
	m := map[string]string{}
	for env, key := range map[string]string{EnvTraceparent: "traceparent", EnvTracestate: "tracestate", EnvBaggage: "baggage"} {
		if v := os.Getenv(env); v != "" {
			m[key] = v
		}
	}
	return ExtractMap(ctx, m)
}
