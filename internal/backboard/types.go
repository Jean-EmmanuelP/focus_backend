package backboard

import "encoding/json"

// ==========================================
// Backboard API Request/Response Types
// ==========================================

// AssistantConfig is the payload for creating/updating an assistant.
type AssistantConfig struct {
	Name        string        `json:"name"`
	SystemPrompt string       `json:"system_prompt"`
	Description string        `json:"description"`
	Tools       []ToolDef     `json:"tools"`
}

// ToolDef defines a tool available to the assistant.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Required    []string               `json:"required,omitempty"`
}

// MessageRequest is the body for POST /threads/{id}/messages.
type MessageRequest struct {
	Content string `json:"content"`
	Stream  bool   `json:"stream"`
	Memory  string `json:"memory"`
}

// MessageResponse is the response from sending a message or submitting tool outputs.
type MessageResponse struct {
	Message           string              `json:"message"`
	ThreadID          string              `json:"thread_id"`
	Content           *string             `json:"content"`
	MessageID         *string             `json:"message_id"`
	Role              *string             `json:"role"`
	Status            *string             `json:"status"` // COMPLETED, REQUIRES_ACTION, IN_PROGRESS, FAILED
	ToolCalls         []ToolCall          `json:"tool_calls,omitempty"`
	RunID             *string             `json:"run_id"`
	MemoryOperationID *string             `json:"memory_operation_id"`
	RetrievedMemories []RetrievedMemory   `json:"retrieved_memories,omitempty"`
}

// ToolCall represents a tool invocation requested by the AI.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     *string      `json:"type,omitempty"`
	Function ToolFunction `json:"function"`
}

// ToolFunction contains the tool name and arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolOutput is the result of executing a tool, submitted back to Backboard.
type ToolOutput struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
}

// SubmitToolOutputsRequest wraps tool outputs for submission.
type SubmitToolOutputsRequest struct {
	ToolOutputs []ToolOutput `json:"tool_outputs"`
}

// Thread represents a Backboard conversation thread.
type Thread struct {
	ThreadID  string           `json:"thread_id"`
	CreatedAt string           `json:"created_at"`
	Messages  []ThreadMessage  `json:"messages,omitempty"`
}

// ThreadMessage is a single message in a thread.
type ThreadMessage struct {
	MessageID *string `json:"message_id,omitempty"`
	Role      string  `json:"role"` // "user" or "assistant"
	Content   *string `json:"content,omitempty"`
	CreatedAt *string `json:"created_at,omitempty"`
}

// Memory represents a stored fact about the user.
type Memory struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Score     *float64               `json:"score,omitempty"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

// RetrievedMemory is a memory returned with a relevance score.
type RetrievedMemory struct {
	ID     string  `json:"id"`
	Memory string  `json:"memory"`
	Score  float64 `json:"score"`
}

// MemoryCreateRequest is the body for adding a memory.
type MemoryCreateRequest struct {
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SideEffect represents an action the frontend should take after an AI response.
type SideEffect struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// NewSideEffect creates a side effect with no data.
func NewSideEffect(typ string) SideEffect {
	return SideEffect{Type: typ}
}

// NewSideEffectWithData creates a side effect with JSON data.
func NewSideEffectWithData(typ string, data interface{}) SideEffect {
	b, _ := json.Marshal(data)
	return SideEffect{Type: typ, Data: b}
}
