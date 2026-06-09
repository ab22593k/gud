package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// initialize sends the ACP initialize handshake and verifies the response.
func (c *acpClient) initialize() error {
	_, err := c.doRequest(acpMethodInitialize, map[string]interface{}{
		"protocolVersion":    1,
		"clientCapabilities": map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    clientName,
			"title":   clientTitle,
			"version": clientVersion,
		},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	return nil
}

// createSession sends session/new and stores the returned session ID.
func (c *acpClient) createSession() error {
	wd, _ := os.Getwd()

	result, err := c.doRequest(acpMethodSessionNew, map[string]interface{}{
		"cwd":        wd,
		"mcpServers": []interface{}{},
	})
	if err != nil {
		return fmt.Errorf("session/new: %w", err)
	}

	var resp struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return fmt.Errorf("failed to parse session/new response: %w", err)
	}
	c.sessionID = resp.SessionID

	return nil
}

// sendPrompt sends a session/prompt request and collects the response,
// handling streaming session/update notifications along the way.
func (c *acpClient) sendPrompt(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	_ = ctx // context is used indirectly via the subprocess lifecycle

	promptText := contentsToPromptText(req.Contents)

	params := map[string]interface{}{
		mapKeySessionID: c.sessionID,
		"prompt": []map[string]interface{}{
			{contentTypeText: promptText, mapKeyType: contentTypeText},
		},
	}

	id, err := c.sendRequest(acpMethodSessionPrompt, params)
	if err != nil {
		return nil, err
	}

	var textBuilder strings.Builder
	for {
		var msg jsonrpcMessage

		if err := c.decoder.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to read prompt response: %w", err)
		}

		if msg.ID == nil {
			collectAgentMessageChunk(msg, &textBuilder)

			continue
		}

		if *msg.ID == id {
			if msg.Error != nil {
				return nil, fmt.Errorf("session/prompt error %d: %s", msg.Error.Code, msg.Error.Message)
			}

			content := genai.NewContentFromText(textBuilder.String(), roleModel)

			return &model.LLMResponse{Content: content}, nil
		}
	}
}

// collectAgentMessageChunk checks if msg is a session/update notification with
// an agent_message_chunk and appends text content to the builder.
func collectAgentMessageChunk(msg jsonrpcMessage, textBuilder *strings.Builder) {
	if msg.Method != acpSessionUpdateEvent || len(msg.Params) == 0 {
		return
	}

	var update struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(msg.Params, &update); err != nil {
		return
	}

	var body struct {
		SessionUpdate string          `json:"sessionUpdate"`
		Content       json.RawMessage `json:"content,omitempty"`
	}
	if err := json.Unmarshal(update.Update, &body); err != nil {
		return
	}

	if body.SessionUpdate != acpSessionUpdateChunk || len(body.Content) == 0 {
		return
	}

	var content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body.Content, &content); err != nil {
		return
	}
	if content.Type == contentTypeText {
		textBuilder.WriteString(content.Text)
	}
}
