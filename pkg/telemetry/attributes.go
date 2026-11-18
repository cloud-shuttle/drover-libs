package telemetry

import "go.opentelemetry.io/otel/attribute"

// Drover correlation attributes. They are the join keys between traces, the
// session event log and audit rows, so keep the names stable.
const (
	KeyTenantID   = attribute.Key("drover.tenant.id")
	KeySessionID  = attribute.Key("drover.session.id")
	KeyRunID      = attribute.Key("drover.run.id")
	KeyTurnIndex  = attribute.Key("drover.turn.index")
	KeyAgentID    = attribute.Key("drover.agent.id")
	KeyWorkflowID = attribute.Key("drover.workflow.id")
	// KeyCostUSD matches drover-gateway's drover.cost.total_usd.
	KeyCostUSD = attribute.Key("drover.cost.total_usd")
)

// OpenTelemetry GenAI semantic convention attributes, as written by
// drover-gateway (deploy/observability/genai-span-attributes.md).
const (
	KeyGenAIOperationName    = attribute.Key("gen_ai.operation.name")
	KeyGenAIProviderName     = attribute.Key("gen_ai.provider.name")
	KeyGenAIRequestModel     = attribute.Key("gen_ai.request.model")
	KeyGenAIResponseModel    = attribute.Key("gen_ai.response.model")
	KeyGenAIUsageInputTokens = attribute.Key("gen_ai.usage.input_tokens")
	// KeyGenAIUsageOutputTokens counts completion tokens.
	KeyGenAIUsageOutputTokens = attribute.Key("gen_ai.usage.output_tokens")
	KeyGenAIUsageCost         = attribute.Key("gen_ai.usage.cost")
	KeyGenAIFinishReason      = attribute.Key("gen_ai.response.finish_reason")
	KeyGenAIConversationID    = attribute.Key("gen_ai.conversation.id")
	KeyGenAIToolName          = attribute.Key("gen_ai.tool.name")
	KeyGenAIToolCallID        = attribute.Key("gen_ai.tool.call.id")
)

// GenAI operation names from the semantic conventions.
const (
	OpChat        = "chat"
	OpExecuteTool = "execute_tool"
	OpInvokeAgent = "invoke_agent"
	OpEmbeddings  = "embeddings"
)
