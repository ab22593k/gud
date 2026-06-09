// Command acpmock is a minimal ACP (Agent Client Protocol) mock server for testing.
// It communicates over JSON-RPC 2.0 via stdio and responds to initialize,
// session/new, and session/prompt requests.
//
// Build: go build -o /dev/null  # checked into testdata for go test
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

func main() {
	// === Handle initialize ===
	req := readRequest()
	if req.Method != "initialize" {
		fatal(fmt.Sprintf("expected initialize, got %s", req.Method))
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

	// === Handle session/new ===
	req = readRequest()
	if req.Method != "session/new" {
		fatal(fmt.Sprintf("expected session/new, got %s", req.Method))
	}
	writeResult(*req.ID, map[string]interface{}{
		"sessionId": "sess_mock_001",
	})

	// === Handle session/prompt (with streaming notification) ===
	req = readRequest()
	if req.Method != "session/prompt" {
		fatal(fmt.Sprintf("expected session/prompt, got %s", req.Method))
	}

	// Send a session/update notification with agent_message_chunk
	writeNotification("session/update", map[string]interface{}{
		"sessionId": "sess_mock_001",
		"update": map[string]interface{}{
			"sessionUpdate": "agent_message_chunk",
			"messageId":     "msg_mock_001",
			"content": map[string]string{
				"type": "text",
				"text": "Generated commit message",
			},
		},
	})

	// Send the session/prompt response with stopReason
	writeResult(*req.ID, map[string]interface{}{
		"stopReason": "end_turn",
	})

	// Close stdin to signal end of input
	os.Exit(0)
}

func readRequest() jsonrpcMessage {
	var msg jsonrpcMessage
	if err := decoder.Decode(&msg); err != nil {
		fatal(fmt.Sprintf("decode error: %v", err))
	}
	if msg.ID == nil {
		fatal("expected request with ID, got notification")
	}
	return msg
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
