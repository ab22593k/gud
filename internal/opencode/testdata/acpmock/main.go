// Command acpmock is a minimal ACP (Agent Client Protocol) mock server for testing.
// It communicates over JSON-RPC 2.0 via stdio and responds to initialize,
// session/new, and multiple session/prompt requests.
//
// It stays alive until stdin is closed, supporting keep-alive tests.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSON-RPC message types matching the client's expectations
type jsonrpcMessage struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var decoder = json.NewDecoder(os.Stdin)
var encoder = json.NewEncoder(os.Stdout)

var promptCount int

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "acpmock: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// === Handle initialize ===
	req, err := readRequest()
	if err != nil {
		return fmt.Errorf("read initialize: %w", err)
	}
	if req.Method != "initialize" {
		return fmt.Errorf("expected initialize, got %s", req.Method)
	}
	writeResult(*req.ID, map[string]interface{}{
		"protocolVersion": 1,
		"agentCapabilities": map[string]interface{}{
			"loadSession": false,
		},
		"agentInfo": map[string]string{
			"name":  "acpmock",
			"title": "ACP Mock Agent",
		},
		"authMethods": []interface{}{},
	})

	// === Handle session/new (only once) ===
	req, err = readRequest()
	if err != nil {
		return fmt.Errorf("read session/new: %w", err)
	}
	if req.Method != "session/new" {
		return fmt.Errorf("expected session/new, got %s", req.Method)
	}
	writeResult(*req.ID, map[string]interface{}{
		"sessionId": "sess_mock_001",
	})

	// === Handle multiple session/prompt requests in a loop ===
	// Keep-alive: the mock stays alive processing prompts until stdin closes.
	for {
		req, err = readRequest()
		if err != nil {
			// Stdin closed — normal shutdown
			return nil
		}
		if req.Method != "session/prompt" {
			return fmt.Errorf("expected session/prompt, got %s", req.Method)
		}

		promptCount++

		// Send a session/update notification with agent_message_chunk
		writeNotification("session/update", map[string]interface{}{
			"sessionId": "sess_mock_001",
			"update": map[string]interface{}{
				"sessionUpdate": "agent_message_chunk",
				"messageId":     fmt.Sprintf("msg_mock_%d", promptCount),
				"content": map[string]string{
					"type": "text",
					"text": fmt.Sprintf("Generated commit message #%d", promptCount),
				},
			},
		})

		// Send the session/prompt response
		writeResult(*req.ID, map[string]interface{}{
			"stopReason": "end_turn",
		})
	}
}

func readRequest() (jsonrpcMessage, error) {
	var msg jsonrpcMessage
	if err := decoder.Decode(&msg); err != nil {
		return msg, fmt.Errorf("decode: %w", err)
	}
	if msg.ID == nil {
		return msg, fmt.Errorf("expected request with ID, got notification")
	}
	return msg, nil
}

func writeResult(id int, result interface{}) {
	msg := jsonrpcMessage{
		Jsonrpc: "2.0",
		ID:      &id,
		Result:  mustMarshal(result),
	}
	if err := encoder.Encode(msg); err != nil {
		fatal(fmt.Sprintf("encode error: %v", err))
	}
}

func writeNotification(method string, params interface{}) {
	msg := jsonrpcMessage{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  mustMarshal(params),
	}
	if err := encoder.Encode(msg); err != nil {
		fatal(fmt.Sprintf("encode notification error: %v", err))
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		fatal(fmt.Sprintf("marshal error: %v", err))
	}
	return data
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "acpmock: %s\n", msg)
	os.Exit(1)
}
