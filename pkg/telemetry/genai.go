package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName is the instrumentation scope for spans this package starts.
const ScopeName = "github.com/cloud-shuttle/drover-libs/pkg/telemetry"

func tracer() trace.Tracer { return otel.Tracer(ScopeName) }

// StartChat starts a client span for one model call, named "chat <model>"
// as the GenAI conventions specify. provider is the gen_ai.provider.name
// value ("anthropic", "openai", "drover-gateway").
func StartChat(ctx context.Context, provider, model string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs,
		KeyGenAIOperationName.String(OpChat),
		KeyGenAIProviderName.String(provider),
		KeyGenAIRequestModel.String(model),
	)
	return tracer().Start(ctx, OpChat+" "+model,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...))
}

// StartTool starts an internal span for one tool call, named
// "execute_tool <name>".
func StartTool(ctx context.Context, name, callID string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs,
		KeyGenAIOperationName.String(OpExecuteTool),
		KeyGenAIToolName.String(name),
	)
	if callID != "" {
		attrs = append(attrs, KeyGenAIToolCallID.String(callID))
	}
	return tracer().Start(ctx, OpExecuteTool+" "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...))
}

// StartAgent starts an internal span for one agent invocation (a run or a
// session turn), named "invoke_agent <agent>".
func StartAgent(ctx context.Context, agent string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs, KeyGenAIOperationName.String(OpInvokeAgent))
	if ids := IDsFromContext(ctx); ids.SessionID != "" {
		attrs = append(attrs, KeyGenAIConversationID.String(ids.SessionID))
	}
	return tracer().Start(ctx, OpInvokeAgent+" "+agent,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...))
}

// Usage is the token and cost accounting for one model call.
type Usage struct {
	InputTokens   int64
	OutputTokens  int64
	CostUSD       float64
	ResponseModel string
	FinishReason  string
}

// RecordUsage sets the usage attributes on span. Zero values are skipped.
func RecordUsage(span trace.Span, u Usage) {
	var attrs []attribute.KeyValue
	if u.InputTokens > 0 {
		attrs = append(attrs, KeyGenAIUsageInputTokens.Int64(u.InputTokens))
	}
	if u.OutputTokens > 0 {
		attrs = append(attrs, KeyGenAIUsageOutputTokens.Int64(u.OutputTokens))
	}
	if u.CostUSD > 0 {
		attrs = append(attrs, KeyGenAIUsageCost.Float64(u.CostUSD), KeyCostUSD.Float64(u.CostUSD))
	}
	if u.ResponseModel != "" {
		attrs = append(attrs, KeyGenAIResponseModel.String(u.ResponseModel))
	}
	if u.FinishReason != "" {
		attrs = append(attrs, KeyGenAIFinishReason.String(u.FinishReason))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

// End records err on span (if non-nil), sets the status and ends it. Use it
// as `defer func() { telemetry.End(span, err) }()`.
func End(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// TraceID returns the hex trace ID of the span in ctx, or "" if there is
// none. Write it into audit rows and session events to join them to traces.
func TraceID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

// SpanID returns the hex span ID of the span in ctx, or "".
func SpanID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.HasSpanID() {
		return sc.SpanID().String()
	}
	return ""
}
