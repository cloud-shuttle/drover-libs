package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// IDs are the Drover correlation IDs for the work a context belongs to.
// Empty fields are omitted from spans. TurnIndex is only written when
// HasTurn is true, so turn 0 is distinguishable from "not a session turn".
type IDs struct {
	TenantID   string
	SessionID  string
	RunID      string
	AgentID    string
	WorkflowID string
	TurnIndex  int
	HasTurn    bool
}

type idsKey struct{}

// WithIDs returns a context carrying ids. Non-empty fields override any IDs
// already on ctx; empty fields keep the outer value, so a turn can add
// TurnIndex under a session context without repeating the tenant.
func WithIDs(ctx context.Context, ids IDs) context.Context {
	merged := IDsFromContext(ctx)
	if ids.TenantID != "" {
		merged.TenantID = ids.TenantID
	}
	if ids.SessionID != "" {
		merged.SessionID = ids.SessionID
	}
	if ids.RunID != "" {
		merged.RunID = ids.RunID
	}
	if ids.AgentID != "" {
		merged.AgentID = ids.AgentID
	}
	if ids.WorkflowID != "" {
		merged.WorkflowID = ids.WorkflowID
	}
	if ids.HasTurn {
		merged.TurnIndex = ids.TurnIndex
		merged.HasTurn = true
	}
	return context.WithValue(ctx, idsKey{}, merged)
}

// IDsFromContext returns the IDs on ctx, or the zero value.
func IDsFromContext(ctx context.Context) IDs {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		return ids
	}
	return IDs{}
}

// Attributes returns the non-empty IDs as span attributes.
func (ids IDs) Attributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 6)
	add := func(k attribute.Key, v string) {
		if v != "" {
			attrs = append(attrs, k.String(v))
		}
	}
	add(KeyTenantID, ids.TenantID)
	add(KeySessionID, ids.SessionID)
	add(KeyRunID, ids.RunID)
	add(KeyAgentID, ids.AgentID)
	add(KeyWorkflowID, ids.WorkflowID)
	if ids.HasTurn {
		attrs = append(attrs, KeyTurnIndex.Int(ids.TurnIndex))
	}
	return attrs
}

// idsProcessor stamps the context's IDs onto every span when it starts.
type idsProcessor struct{}

// NewIDsProcessor returns the span processor Setup installs. It is exported
// for services that build their own TracerProvider.
func NewIDsProcessor() sdktrace.SpanProcessor { return idsProcessor{} }

func (idsProcessor) OnStart(ctx context.Context, s sdktrace.ReadWriteSpan) {
	if attrs := IDsFromContext(ctx).Attributes(); len(attrs) > 0 {
		s.SetAttributes(attrs...)
	}
}
func (idsProcessor) OnEnd(sdktrace.ReadOnlySpan)      {}
func (idsProcessor) Shutdown(context.Context) error   { return nil }
func (idsProcessor) ForceFlush(context.Context) error { return nil }
