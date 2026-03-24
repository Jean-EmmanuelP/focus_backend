package backboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the HTTP client for the Backboard API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Backboard API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: "https://app.backboard.io/api",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Backboard can be slow due to LLM + tool calls
		},
	}
}

// ==========================================
// Assistant Management
// ==========================================

// CreateAssistant creates a new assistant and returns its ID.
func (c *Client) CreateAssistant(ctx context.Context, config AssistantConfig) (string, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal assistant config: %w", err)
	}

	resp, err := c.do(ctx, "POST", "/assistants", body)
	if err != nil {
		return "", fmt.Errorf("create assistant: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse create assistant response: %w", err)
	}

	id, ok := result["assistant_id"].(string)
	if !ok {
		return "", fmt.Errorf("missing assistant_id in response")
	}
	return id, nil
}

// UpdateAssistant patches an existing assistant with new config.
func (c *Client) UpdateAssistant(ctx context.Context, assistantID string, config AssistantConfig) error {
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal assistant config: %w", err)
	}

	_, err = c.do(ctx, "PATCH", "/assistants/"+assistantID, body)
	if err != nil {
		return fmt.Errorf("update assistant: %w", err)
	}
	return nil
}

// ==========================================
// Thread Management
// ==========================================

// CreateThread creates a new conversation thread for an assistant.
func (c *Client) CreateThread(ctx context.Context, assistantID string) (string, error) {
	resp, err := c.do(ctx, "POST", "/assistants/"+assistantID+"/threads", []byte("{}"))
	if err != nil {
		return "", fmt.Errorf("create thread: %w", err)
	}

	var thread Thread
	if err := json.Unmarshal(resp, &thread); err != nil {
		return "", fmt.Errorf("parse thread: %w", err)
	}
	return thread.ThreadID, nil
}

// ListThreads returns all threads for an assistant.
func (c *Client) ListThreads(ctx context.Context, assistantID string) ([]Thread, error) {
	resp, err := c.do(ctx, "GET", "/assistants/"+assistantID+"/threads", nil)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}

	var threads []Thread
	if err := json.Unmarshal(resp, &threads); err != nil {
		return nil, fmt.Errorf("parse threads: %w", err)
	}
	return threads, nil
}

// GetThread returns a thread with its messages.
func (c *Client) GetThread(ctx context.Context, threadID string) (*Thread, error) {
	resp, err := c.do(ctx, "GET", "/threads/"+threadID, nil)
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}

	var thread Thread
	if err := json.Unmarshal(resp, &thread); err != nil {
		return nil, fmt.Errorf("parse thread: %w", err)
	}
	return &thread, nil
}

// DeleteThread deletes a conversation thread.
func (c *Client) DeleteThread(ctx context.Context, threadID string) error {
	_, err := c.do(ctx, "DELETE", "/threads/"+threadID, nil)
	return err
}

// ==========================================
// Messages
// ==========================================

// SendMessage sends a user message to a thread and returns the AI response.
func (c *Client) SendMessage(ctx context.Context, threadID string, content string) (*MessageResponse, error) {
	req := MessageRequest{
		Content: content,
		Stream:  false,
		Memory:  "Readonly",
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	resp, err := c.do(ctx, "POST", "/threads/"+threadID+"/messages", body)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	var msgResp MessageResponse
	if err := json.Unmarshal(resp, &msgResp); err != nil {
		return nil, fmt.Errorf("parse message response: %w", err)
	}
	return &msgResp, nil
}

// SubmitToolOutputs submits tool execution results and returns the next response.
func (c *Client) SubmitToolOutputs(ctx context.Context, threadID, runID string, outputs []ToolOutput) (*MessageResponse, error) {
	req := SubmitToolOutputsRequest{ToolOutputs: outputs}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal tool outputs: %w", err)
	}

	path := fmt.Sprintf("/threads/%s/runs/%s/submit-tool-outputs", threadID, runID)
	resp, err := c.do(ctx, "POST", path, body)
	if err != nil {
		return nil, fmt.Errorf("submit tool outputs: %w", err)
	}

	var msgResp MessageResponse
	if err := json.Unmarshal(resp, &msgResp); err != nil {
		return nil, fmt.Errorf("parse tool output response: %w", err)
	}
	return &msgResp, nil
}

// ==========================================
// Memories
// ==========================================

// ListMemories returns all memories for an assistant.
func (c *Client) ListMemories(ctx context.Context, assistantID string) ([]Memory, error) {
	resp, err := c.do(ctx, "GET", "/assistants/"+assistantID+"/memories", nil)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}

	var result struct {
		Memories []Memory `json:"memories"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		// Fallback: try parsing as plain array
		var memories []Memory
		if err2 := json.Unmarshal(resp, &memories); err2 != nil {
			return nil, fmt.Errorf("parse memories: %w", err)
		}
		return memories, nil
	}
	return result.Memories, nil
}

// AddMemory stores a new memory for the assistant.
func (c *Client) AddMemory(ctx context.Context, assistantID string, content string) error {
	req := MemoryCreateRequest{Content: content, Metadata: nil}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
	}

	_, err = c.do(ctx, "POST", "/assistants/"+assistantID+"/memories", body)
	return err
}

// DeleteMemory removes a specific memory.
func (c *Client) DeleteMemory(ctx context.Context, assistantID, memoryID string) error {
	_, err := c.do(ctx, "DELETE", "/assistants/"+assistantID+"/memories/"+memoryID, nil)
	return err
}

// ==========================================
// HTTP Helper
// ==========================================

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backboard %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
