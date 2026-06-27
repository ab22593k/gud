package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// initialize sends the ACP initialize handshake and verifies the response.
func (c *acpClient) initialize(ctx context.Context) error {
	_, err := c.doRequest(ctx, acpMethodInitialize, map[string]interface{}{
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
func (c *acpClient) createSession(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("session/new: getwd: %w", err)
	}

	result, err := c.doRequest(ctx, acpMethodSessionNew, map[string]interface{}{
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
	promptText := contentsToPromptText(req.Contents)

	params := map[string]interface{}{
		mapKeySessionID: c.sessionID,
		"model":         req.Model,
		"prompt": []map[string]string{
			{mapKeyType: contentTypeText, contentTypeText: promptText},
		},
	}

	id, err := c.sendRequest(acpMethodSessionPrompt, params)
	if err != nil {
		return nil, err
	}

	var textBuilder strings.Builder
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r, ok := <-c.decodeCh:
			if !ok {
				return nil, errors.New("acp client: connection closed")
			}
			if r.err != nil {
				return nil, fmt.Errorf("failed to read prompt response: %w", r.err)
			}

			if r.msg.ID == nil {
				collectAgentMessageChunk(r.msg, &textBuilder)

				continue
			}

			if *r.msg.ID == id {
				if r.msg.Error != nil {
					return nil, fmt.Errorf("session/prompt error %d: %s", r.msg.Error.Code, r.msg.Error.Message)
				}

				content := genai.NewContentFromText(textBuilder.String(), roleModel)

				return &model.LLMResponse{Content: content}, nil
			}
		}
	}
}

// cancelPrompt sends a session/cancel notification to abort the current prompt.
func (c *acpClient) cancelPrompt() error {
	return c.sendNotification(acpMethodSessionCancel, map[string]interface{}{
		mapKeySessionID: c.sessionID,
	})
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
