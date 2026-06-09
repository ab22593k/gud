package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"strings"
	"sync"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

const defaultModel = "deepseek-v4-flash"

// Config holds configuration for the OpenCode.ai ACP client.
type Config struct {
	APIKey   string // Passed as env var to the subprocess
	Model    string // Model name for identification
	BasePath string // Path to opencode binary (default: "opencode")
}

// Model implements model.LLM using OpenCode.ai's ACP protocol.
// It spawns `opencode acp` as a subprocess and communicates via JSON-RPC over stdio.
type Model struct {
	config Config
	client *acpClient
	mu     sync.Mutex
}

// NewModel creates a new OpenCode.ai ACP model client.
func NewModel(cfg Config) *Model {
	if cfg.BasePath == "" {
		cfg.BasePath = "opencode"
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return &Model{config: cfg}
}

// Name returns the configured model name.
func (m *Model) Name() string { return m.config.Model }

// GenerateContent implements model.LLM by sending a prompt to the ACP agent.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		client, err := m.getOrStartClient(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		resp, err := client.sendPrompt(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(resp, nil)
	}
}

func (m *Model) getOrStartClient(ctx context.Context) (*acpClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil && m.client.alive() {
		return m.client, nil
	}

	if m.client != nil {
		m.client.close()
	}

	client, err := startACPClient(ctx, m.config)
	if err != nil {
		return nil, err
	}
	m.client = client
	return client, nil
}

// ---------------------------------------------------------------------------
// ACP Client — JSON-RPC 2.0 over stdio with `opencode acp`
// ---------------------------------------------------------------------------

type acpClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	decoder   *json.Decoder
	nextID    int
	sessionID string
	done      chan struct{} // closed by Wait() goroutine when process exits
	closeOnce sync.Once
}

// startACPClient spawns `opencode acp`, initializes the ACP connection,
// and creates a new session.
func startACPClient(ctx context.Context, cfg Config) (*acpClient, error) {
	cmd := exec.CommandContext(ctx, cfg.BasePath, "acp")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	// Use the user's existing opencode configuration for auth.
	// Only pass OPENCODE_API_KEY if explicitly provided to override.
	cmd.Env = os.Environ()
	if cfg.APIKey != "" {
		cmd.Env = append(cmd.Env, "OPENCODE_API_KEY="+cfg.APIKey)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to start %q acp: %w (is opencode installed?)", cfg.BasePath, err)
	}

	c := &acpClient{
		cmd:     cmd,
		stdin:   stdin,
		decoder: json.NewDecoder(stdout),
		done:    make(chan struct{}),
	}

	// Monitor process exit in a goroutine so alive() works correctly
	// even when the process crashes or exits unexpectedly.
	go func() {
		cmd.Wait()
		close(c.done)
	}()

	if err := c.initialize(); err != nil {
		c.close()
		return nil, err
	}
	if err := c.createSession(); err != nil {
		c.close()
		return nil, err
	}

	return c, nil
}

func (c *acpClient) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *acpClient) close() {
	c.closeOnce.Do(func() {
		if c.stdin != nil {
			c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		// Wait for the goroutine to finish, preventing zombie processes.
		<-c.done
	})
}

// ---------------------------------------------------------------------------
// JSON-RPC message types
// ---------------------------------------------------------------------------

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

// sendRequest encodes and writes a JSON-RPC request to the subprocess stdin.
// Returns the request ID so the caller can match the response.
func (c *acpClient) sendRequest(method string, params interface{}) (int, error) {
	c.nextID++
	id := c.nextID

	msg := jsonrpcMessage{
		Jsonrpc: "2.0",
		ID:      &id,
		Method:  method,
		Params:  mustMarshal(params),
	}

	if err := json.NewEncoder(c.stdin).Encode(msg); err != nil {
		return 0, fmt.Errorf("failed to send %s request: %w", method, err)
	}

	return id, nil
}

// readResponse reads JSON-RPC messages from stdout until a response with the
// expected ID arrives. Notifications (messages without an ID) are forwarded
// to handleNotification.
func (c *acpClient) readResponse(expectedID int) (json.RawMessage, error) {
	for {
		var msg jsonrpcMessage
		if err := c.decoder.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if msg.ID == nil {
			// JSON-RPC notification — ignore for simple request/response
			continue
		}

		if *msg.ID != expectedID {
			continue
		}

		if msg.Error != nil {
			return nil, fmt.Errorf("RPC error %d: %s", msg.Error.Code, msg.Error.Message)
		}

		return msg.Result, nil
	}
}

// doRequest is a simple request/response helper (no notification handling).
func (c *acpClient) doRequest(method string, params interface{}) (json.RawMessage, error) {
	id, err := c.sendRequest(method, params)
	if err != nil {
		return nil, err
	}
	return c.readResponse(id)
}

// ---------------------------------------------------------------------------
// ACP lifecycle methods
// ---------------------------------------------------------------------------

// initialize sends the initialize handshake and verifies the response.
func (c *acpClient) initialize() error {
	_, err := c.doRequest("initialize", map[string]interface{}{
		"protocolVersion":   1,
		"clientCapabilities": map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "gud",
			"title":   "Gud Commit Message Generator",
			"version": "0.1.0",
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

	result, err := c.doRequest("session/new", map[string]interface{}{
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
		"sessionId": c.sessionID,
		"prompt": []map[string]interface{}{
			{"type": "text", "text": promptText},
		},
	}

	id, err := c.sendRequest("session/prompt", params)
	if err != nil {
		return nil, err
	}

	// Read messages until we get the session/prompt response.
	// Along the way, collect agent_message_chunk text from session/update notifications.
	var textBuilder strings.Builder
	for {
		var msg jsonrpcMessage
		if err := c.decoder.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to read prompt response: %w", err)
		}

		// Notification — check for agent_message_chunk
		if msg.ID == nil {
			collectAgentMessageChunk(msg, &textBuilder)
			continue
		}

		// Response to our session/prompt request
		if *msg.ID == id {
			if msg.Error != nil {
				return nil, fmt.Errorf("session/prompt error %d: %s", msg.Error.Code, msg.Error.Message)
			}
			content := genai.NewContentFromText(textBuilder.String(), "model")
			return &model.LLMResponse{Content: content}, nil
		}
		// Response for a different request — ignore
	}
}

// collectAgentMessageChunk checks if msg is a session/update notification with
// an agent_message_chunk and appends text content to the builder.
func collectAgentMessageChunk(msg jsonrpcMessage, textBuilder *strings.Builder) {
	if msg.Method != "session/update" || len(msg.Params) == 0 {
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

	if body.SessionUpdate != "agent_message_chunk" || len(body.Content) == 0 {
		return
	}

	var content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body.Content, &content); err != nil {
		return
	}
	if content.Type == "text" {
		textBuilder.WriteString(content.Text)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal: %v", err))
	}
	return data
}
