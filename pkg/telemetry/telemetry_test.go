package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupInMemory runs Setup with in-memory exporters and restores the
// globals when the test ends. Tests that call it must not run in parallel.
func setupInMemory(t *testing.T, ratio float64) *tracetest.InMemoryExporter {
	t.Helper()
	prevTP, prevMP, prevProp := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	exp := tracetest.NewInMemoryExporter()
	shutdown, err := Setup(context.Background(), Config{
		ServiceName:   "test-svc",
		Enabled:       true,
		SampleRatio:   ratio,
		TraceExporter: exp,
		MetricReader:  sdkmetric.NewManualReader(),
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})
	return exp
}

func flush(t *testing.T) {
	t.Helper()
	tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	if !ok {
		t.Fatalf("global tracer provider is %T, want *sdktrace.TracerProvider", otel.GetTracerProvider())
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
}

func attrMap(kvs []attribute.KeyValue) map[attribute.Key]attribute.Value {
	m := make(map[attribute.Key]attribute.Value, len(kvs))
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestConfigFromEnv(t *testing.T) {
	for _, k := range []string{EnvEnabled, EnvEndpoint, EnvEnvironment, "ENVIRONMENT", "OTEL_SDK_DISABLED", "OTEL_SERVICE_NAME",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_TRACES_SAMPLER_ARG"} {
		t.Setenv(k, "")
	}

	cfg := ConfigFromEnv("svc", "1.2.3")
	if cfg.Enabled || cfg.ServiceName != "svc" || cfg.ServiceVersion != "1.2.3" || cfg.Environment != "development" || cfg.SampleRatio != 1 {
		t.Fatalf("defaults: %+v", cfg)
	}

	t.Setenv(EnvEndpoint, "collector:4317")
	if cfg = ConfigFromEnv("svc", ""); !cfg.Enabled || cfg.Endpoint != "collector:4317" {
		t.Fatalf("DROVER_OTEL_ENDPOINT should enable export: %+v", cfg)
	}

	t.Setenv(EnvEnabled, "false")
	if cfg = ConfigFromEnv("svc", ""); cfg.Enabled {
		t.Fatalf("DROVER_OTEL_ENABLED=false should win over an endpoint: %+v", cfg)
	}

	t.Setenv(EnvEnabled, "1")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://otel.example:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", ProtocolHTTP)
	t.Setenv("OTEL_SERVICE_NAME", "renamed")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	t.Setenv(EnvEnvironment, "staging")
	cfg = ConfigFromEnv("svc", "")
	if !cfg.Enabled || cfg.Endpoint != "" || cfg.Protocol != ProtocolHTTP || cfg.ServiceName != "renamed" || cfg.SampleRatio != 0.25 || cfg.Environment != "staging" {
		t.Fatalf("OTEL_* overrides: %+v", cfg)
	}

	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "7")
	if cfg = ConfigFromEnv("svc", ""); cfg.SampleRatio != 1 {
		t.Fatalf("out-of-range sampler arg should be ignored, got %v", cfg.SampleRatio)
	}

	t.Setenv("OTEL_SDK_DISABLED", "true")
	if cfg = ConfigFromEnv("svc", ""); cfg.Enabled {
		t.Fatalf("OTEL_SDK_DISABLED=true should disable: %+v", cfg)
	}
}

func TestSetupDisabledStillPropagates(t *testing.T) {
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	shutdown, err := Setup(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Setup disabled: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	in := map[string]string{"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
	ctx := ExtractMap(context.Background(), in)
	if got := TraceID(ctx); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("TraceID after extract = %q", got)
	}
	if out := InjectMap(ctx); out["traceparent"] != in["traceparent"] {
		t.Fatalf("disabled service must pass traceparent through unchanged, got %q", out["traceparent"])
	}
}

func TestSetupRejectsBadConfig(t *testing.T) {
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })
	ctx := context.Background()
	cases := map[string]Config{
		"no service name": {Enabled: true},
		"ratio above 1":   {Enabled: true, ServiceName: "s", SampleRatio: 1.5},
		"bad protocol":    {Enabled: true, ServiceName: "s", Protocol: "carrier-pigeon"},
	}
	for name, cfg := range cases {
		if _, err := Setup(ctx, cfg); err == nil {
			t.Errorf("%s: Setup returned no error", name)
		}
	}
}

func TestSpansCarryDroverIDs(t *testing.T) {
	exp := setupInMemory(t, 1)

	ctx := WithIDs(context.Background(), IDs{TenantID: "t1", SessionID: "s1"})
	ctx = WithIDs(ctx, IDs{TurnIndex: 0, HasTurn: true, RunID: "r9"})
	_, span := otel.Tracer("test").Start(ctx, "turn")
	span.End()
	flush(t)

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	got := attrMap(spans[0].Attributes)
	want := map[attribute.Key]attribute.Value{
		KeyTenantID:  attribute.StringValue("t1"),
		KeySessionID: attribute.StringValue("s1"),
		KeyRunID:     attribute.StringValue("r9"),
		KeyTurnIndex: attribute.IntValue(0),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k].Emit(), v.Emit())
		}
	}
	if _, ok := got[KeyAgentID]; ok {
		t.Errorf("empty AgentID should not be written")
	}
	var svc string
	for _, kv := range spans[0].Resource.Attributes() {
		if kv.Key == "service.name" {
			svc = kv.Value.AsString()
		}
	}
	if svc != "test-svc" {
		t.Errorf("service.name = %q, want test-svc", svc)
	}
}

func TestGenAISpans(t *testing.T) {
	exp := setupInMemory(t, 1)
	ctx := WithIDs(context.Background(), IDs{SessionID: "sess-1"})

	ctx, agent := StartAgent(ctx, "planner")
	cctx, chat := StartChat(ctx, "drover-gateway", "claude-sonnet")
	RecordUsage(chat, Usage{InputTokens: 120, OutputTokens: 40, CostUSD: 0.002, ResponseModel: "claude-sonnet-4", FinishReason: "stop"})
	End(chat, nil)
	_, tool := StartTool(cctx, "git.push", "call-7")
	End(tool, errors.New("remote rejected"))
	End(agent, nil)
	flush(t)

	byName := map[string]tracetest.SpanStub{}
	for _, s := range exp.GetSpans() {
		byName[s.Name] = s
	}
	chatSpan, ok := byName["chat claude-sonnet"]
	if !ok {
		t.Fatalf("no chat span; got %v", names(exp.GetSpans()))
	}
	if chatSpan.SpanKind != trace.SpanKindClient {
		t.Errorf("chat span kind = %v, want client", chatSpan.SpanKind)
	}
	ca := attrMap(chatSpan.Attributes)
	checks := map[attribute.Key]attribute.Value{
		KeyGenAIOperationName:     attribute.StringValue(OpChat),
		KeyGenAIProviderName:      attribute.StringValue("drover-gateway"),
		KeyGenAIRequestModel:      attribute.StringValue("claude-sonnet"),
		KeyGenAIResponseModel:     attribute.StringValue("claude-sonnet-4"),
		KeyGenAIUsageInputTokens:  attribute.Int64Value(120),
		KeyGenAIUsageOutputTokens: attribute.Int64Value(40),
		KeyGenAIUsageCost:         attribute.Float64Value(0.002),
		KeyCostUSD:                attribute.Float64Value(0.002),
		KeyGenAIFinishReason:      attribute.StringValue("stop"),
		KeySessionID:              attribute.StringValue("sess-1"),
	}
	for k, v := range checks {
		if ca[k] != v {
			t.Errorf("chat %s = %v, want %v", k, ca[k].Emit(), v.Emit())
		}
	}

	agentSpan := byName["invoke_agent planner"]
	if attrMap(agentSpan.Attributes)[KeyGenAIConversationID] != attribute.StringValue("sess-1") {
		t.Errorf("agent span missing gen_ai.conversation.id")
	}
	if chatSpan.Parent.SpanID() != agentSpan.SpanContext.SpanID() {
		t.Errorf("chat span is not a child of the agent span")
	}

	toolSpan := byName["execute_tool git.push"]
	if toolSpan.Status.Code != codes.Error || !strings.Contains(toolSpan.Status.Description, "remote rejected") {
		t.Errorf("tool span status = %+v, want error", toolSpan.Status)
	}
	if len(toolSpan.Events) == 0 || toolSpan.Events[0].Name != "exception" {
		t.Errorf("tool span should record the error as an exception event")
	}
	if attrMap(toolSpan.Attributes)[KeyGenAIToolCallID] != attribute.StringValue("call-7") {
		t.Errorf("tool span missing gen_ai.tool.call.id")
	}
	if toolSpan.SpanContext.TraceID() != agentSpan.SpanContext.TraceID() {
		t.Errorf("tool span left the agent's trace")
	}
}

func TestParentBasedSampling(t *testing.T) {
	exp := setupInMemory(t, 0)

	_, root := otel.Tracer("test").Start(context.Background(), "unsampled root")
	if root.SpanContext().IsSampled() {
		t.Fatalf("ratio 0 root span was sampled")
	}
	root.End()

	parent := map[string]string{"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}
	ctx := ExtractMap(context.Background(), parent)
	ctx, child := otel.Tracer("test").Start(ctx, "child of sampled remote")
	if !child.SpanContext().IsSampled() {
		t.Fatalf("child of a sampled remote parent must be sampled")
	}
	if got := TraceID(ctx); got != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("child trace ID = %s, want the caller's", got)
	}
	if SpanID(ctx) == "" || SpanID(ctx) == "b7ad6b7169203331" {
		t.Fatalf("child span ID = %q, want a new span ID", SpanID(ctx))
	}
	child.End()
	flush(t)
	if n := len(exp.GetSpans()); n != 1 {
		t.Fatalf("exported %d spans, want only the sampled child", n)
	}
}

func TestEnvironCarriesTraceToChildProcess(t *testing.T) {
	setupInMemory(t, 1)
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	env := Environ(ctx)
	var tp string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, EnvTraceparent+"="); ok {
			tp = v
		}
	}
	if !strings.Contains(tp, TraceID(ctx)) {
		t.Fatalf("Environ() = %v, want TRACEPARENT with trace %s", env, TraceID(ctx))
	}

	t.Setenv(EnvTraceparent, tp)
	t.Setenv(EnvTracestate, "")
	t.Setenv(EnvBaggage, "")
	child := ContextFromEnviron(context.Background())
	if TraceID(child) != TraceID(ctx) {
		t.Fatalf("ContextFromEnviron trace = %q, want %q", TraceID(child), TraceID(ctx))
	}
}

func TestNoSpanHelpers(t *testing.T) {
	ctx := context.Background()
	if TraceID(ctx) != "" || SpanID(ctx) != "" {
		t.Fatalf("TraceID/SpanID on a bare context should be empty")
	}
	if ExtractMap(ctx, nil) != ctx {
		t.Fatalf("ExtractMap(nil) should return ctx unchanged")
	}
	if got := IDsFromContext(ctx); got != (IDs{}) {
		t.Fatalf("IDsFromContext on a bare context = %+v", got)
	}
}

func names(spans tracetest.SpanStubs) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Name
	}
	return out
}

func TestSetupBuildsOTLPExporters(t *testing.T) {
	prevTP, prevMP, prevProp := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})
	for _, proto := range []string{ProtocolGRPC, ProtocolHTTP} {
		// Nothing listens on port 1; exporters connect lazily, so Setup
		// succeeds and Shutdown gives up quickly.
		shutdown, err := Setup(context.Background(), Config{
			ServiceName: "svc", Enabled: true, SampleRatio: 1,
			Protocol: proto, Endpoint: "127.0.0.1:1",
		})
		if err != nil {
			t.Fatalf("%s: Setup: %v", proto, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = shutdown(ctx)
		cancel()
	}
}
