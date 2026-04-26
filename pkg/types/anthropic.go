// Package types defines shared data structures and interfaces.
package types

import (
	"encoding/json"
	"fmt"
)

// Anthropic API types for the Messages API.
// Reference: https://docs.anthropic.com/en/api/messages

// MessageRequest represents a request to the Anthropic Messages API.
type MessageRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	System      json.RawMessage `json:"system,omitempty"`
	Messages    []Message       `json:"messages"`
	Stream      *bool           `json:"stream,omitempty"`
	Tools       []Tool          `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Metadata    *Metadata       `json:"metadata,omitempty"`
}

// SystemText extracts the system prompt text from the raw system field.
// Anthropic accepts system as either a string or an array of content blocks:
//
//	"system": "You are helpful"
//	"system": [{"type":"text","text":"You are helpful","cache_control":...}]
func (r *MessageRequest) SystemText() string {
	if len(r.System) == 0 {
		return ""
	}
	// Try string first
	var s string
	if err := json.Unmarshal(r.System, &s); err == nil {
		return s
	}
	// Try array of content blocks
	var blocks []SystemContentBlock
	if err := json.Unmarshal(r.System, &blocks); err == nil {
		var text string
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		return text
	}
	return string(r.System)
}

// SystemContentBlock represents a content block in the system array.
type SystemContentBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// CacheControl represents cache control directives.
type CacheControl struct {
	Type string `json:"type"`
}

// Metadata contains optional metadata for the request.
type Metadata struct {
	UserID string `json:"user_id,omitempty"`
}

// Message represents a single message in the conversation.
// Content can be a string or an array of content blocks.
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ContentBlocks parses the message content into content blocks.
// Handles both string content and array-of-blocks content:
//
//	"content": "hello"
//	"content": [{"type":"text","text":"hello"}]
func (m *Message) ContentBlocks() []ContentBlock {
	if len(m.Content) == 0 {
		return nil
	}

	// Try string first
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return []ContentBlock{{Type: "text", Text: s}}
	}

	// Try array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err == nil {
		return blocks
	}

	return nil
}

// ContentBlock represents a block of content within a message.
// The Type field determines which other fields are populated:
//   - "text": Text is populated
//   - "tool_use": ID, Name, Input are populated
//   - "tool_result": ToolUseID, Content, IsError are populated
//   - "thinking": Thinking, Signature are populated
//   - "image": Source is populated
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`          // For tool_use (the tool call ID)
	ToolUseID string          `json:"tool_use_id,omitempty"` // For tool_result (references the tool_use ID)
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`    // Deprecated: use Content
	Content   json.RawMessage `json:"content,omitempty"`   // For tool_result inner content
	IsError   *bool           `json:"is_error,omitempty"`  // For tool_result
	Thinking  string          `json:"thinking,omitempty"`  // For thinking blocks
	Signature string          `json:"signature,omitempty"` // For thinking blocks
	Source    *ImageSource    `json:"source,omitempty"`    // For image blocks
}

// GetToolID returns the appropriate tool ID for this block type.
// For tool_use: returns ID. For tool_result: returns ToolUseID.
func (b *ContentBlock) GetToolID() string {
	if b.Type == "tool_result" {
		return b.ToolUseID
	}
	return b.ID
}

// TextContent extracts text from a tool_result's content field.
// The content field can be a string or an array of content blocks.
func (b *ContentBlock) TextContent() string {
	// For tool_result, content can be string or array
	if len(b.Content) > 0 {
		var s string
		if err := json.Unmarshal(b.Content, &s); err == nil {
			return s
		}
		var blocks []ContentBlock
		if err := json.Unmarshal(b.Content, &blocks); err == nil {
			var text string
			for _, inner := range blocks {
				if inner.Type == "text" {
					text += inner.Text
				}
			}
			return text
		}
	}
	// Fallback to Output field (deprecated but still used)
	if len(b.Output) > 0 {
		var s string
		if err := json.Unmarshal(b.Output, &s); err == nil {
			return s
		}
		return string(b.Output)
	}
	return ""
}

// ImageSource represents an image source for content blocks.
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// Tool represents a tool definition for function calling.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// MessageResponse represents a response from the Anthropic Messages API.
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ContentBlockDelta represents a streaming delta for a content block.
type ContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta Delta  `json:"delta"`
}

// Delta represents a partial update in a streaming response.
type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// MessageEvent represents a Server-Sent Event from the streaming API.
type MessageEvent struct {
	Type    string           `json:"type"`
	Message *MessageResponse `json:"message,omitempty"`
	Index   *int             `json:"index,omitempty"`
	Delta   *Delta           `json:"delta,omitempty"`
	Usage   *Usage           `json:"usage,omitempty"`
	Error   *APIError        `json:"error,omitempty"`
}

// APIError represents an error from the Anthropic API.
type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Validate checks that the MessageRequest has required fields.
func (r *MessageRequest) Validate() error {
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("messages is required")
	}
	return nil
}
