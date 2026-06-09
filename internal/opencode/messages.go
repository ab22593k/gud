package opencode

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// JSON-RPC protocol constants.
const (
	jsonrpcVersion = "2.0"

	// ACP protocol method names.
	acpMethodInitialize    = "initialize"
	acpMethodSessionNew    = "session/new"
	acpMethodSessionPrompt = "session/prompt"

	// ACP session update type for streaming text.
	acpSessionUpdateChunk = "agent_message_chunk"
	acpSessionUpdateEvent = "session/update"

	// Content type identifier.
	contentTypeText = "text"

	// AI role identifier for generated content.
	roleModel = "model"

	// Map keys used in ACP request payloads.
	mapKeySessionID = "sessionId"
	mapKeyType      = "type"

	// ACP client info.
	clientName    = "gud"
	clientTitle   = "Gud Commit Message Generator"
	clientVersion = "0.1.0"
)

// jsonrpcMessage represents a JSON-RPC 2.0 message.
type jsonrpcMessage struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// contentsToPromptText extracts the combined text from genai Content parts.
func contentsToPromptText(contents []*genai.Content) string {
	var sb strings.Builder

	for _, c := range contents {
		if c == nil {
			continue
		}
		for _, part := range c.Parts {
			if part == nil {
				continue
			}
			sb.WriteString(part.Text)
		}
	}

	return sb.String()
}

// mustMarshal panics on json.Marshal failure. It is safe to use for
// fixed structs whose shape is known at compile time.
func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal: %v", err))
	}

	return data
}
