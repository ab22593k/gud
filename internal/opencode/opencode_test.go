package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// ---------------------------------------------------------------------------
// Unit tests (no subprocess needed)
// ---------------------------------------------------------------------------

func TestContentsToPromptText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		contents []*genai.Content
		want     string
	}{
		{
			name: "single text part",
			contents: []*genai.Content{
				genai.NewContentFromText("hello world", "user"),
			},
			want: "hello world",
		},
		{
			name: "multiple contents",
			contents: []*genai.Content{
				genai.NewContentFromText("part one", "user"),
				genai.NewContentFromText("part two", "model"),
			},
			want: "part onepart two",
		},
		{
			name:     "empty contents",
			contents: []*genai.Content{},
			want:     "",
		},
		{
			name: "content with nil parts",
			contents: []*genai.Content{
				{Role: "user", Parts: []*genai.Part{nil, {Text: "text"}, nil}},
			},
			want: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := contentsToPromptText(tt.contents)
			if got != tt.want {
				t.Errorf("contentsToPromptText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectAgentMessageChunk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		msg           jsonrpcMessage
		wantCollected string
	}{
		{
			name: "agent_message_chunk with text",
			msg: jsonrpcMessage{
				Method: "session/update",
				Params: mustMarshal(map[string]interface{}{
					"sessionId": "sess_1",
					"update": map[string]interface{}{
						"sessionUpdate": "agent_message_chunk",
						"messageId":     "msg_1",
						"content": map[string]string{
							"type": "text",
							"text": "Hello ",
						},
					},
				}),
			},
			wantCollected: "Hello ",
		},
		{
			name: "non-text content type is ignored",
			msg: jsonrpcMessage{
				Method: "session/update",
				Params: mustMarshal(map[string]interface{}{
					"sessionId": "sess_1",
					"update": map[string]interface{}{
						"sessionUpdate": "agent_message_chunk",
						"content": map[string]string{
							"type": "image",
							"text": "should not appear",
						},
					},
				}),
			},
			wantCollected: "",
		},
		{
			name: "non-agent_message_chunk update is ignored",
			msg: jsonrpcMessage{
				Method: "session/update",
				Params: mustMarshal(map[string]interface{}{
					"sessionId": "sess_1",
					"update": map[string]interface{}{
						"sessionUpdate": "plan",
						"content": map[string]string{
							"type": "text",
							"text": "plan text",
						},
					},
				}),
			},
			wantCollected: "",
		},
		{
			name: "wrong method is ignored",
			msg: jsonrpcMessage{
				Method: "other_method",
				Params: mustMarshal(map[string]interface{}{}),
			},
			wantCollected: "",
		},
		{
			name:          "nil params",
			msg:           jsonrpcMessage{Method: "session/update"},
			wantCollected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var sb strings.Builder
			collectAgentMessageChunk(tt.msg, &sb)
			if sb.String() != tt.wantCollected {
				t.Errorf("collected %q, want %q", sb.String(), tt.wantCollected)
			}
		})
	}
}

func TestNewModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      Config
		wantName string
	}{
		{
			name:     "default model name",
			cfg:      Config{APIKey: "key"},
			wantName: defaultModel,
		},
		{
			name:     "custom model name",
			cfg:      Config{APIKey: "key", Model: "custom-model"},
			wantName: "custom-model",
		},
		{
			name:     "default base path",
			cfg:      Config{APIKey: "key"},
			wantName: defaultModel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewModel(tt.cfg)
			if m.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", m.Name(), tt.wantName)
			}
			if m.config.BasePath == "" {
				t.Error("BasePath should default to 'opencode'")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests (using mock ACP binary)
// ---------------------------------------------------------------------------

// Global temp dir for the mock binary, shared across all tests in this package.
// Using os.MkdirTemp instead of tb.TempDir() so the binary survives individual
// test cleanup and can be reused via sync.Once.
var (
	acpMockPath string
	acpMockOnce sync.Once
	acpMockErr  error
)

func buildACPMock(tb testing.TB) string {
	tb.Helper()
	acpMockOnce.Do(func() {
		dir, err := os.MkdirTemp("", "acpmock-*")
		if err != nil {
			acpMockErr = fmt.Errorf("create temp dir: %w", err)
			return
		}
		src := filepath.Join("testdata", "acpmock", "main.go")
		dest := filepath.Join(dir, "acpmock")
		cmd := exec.Command("go", "build", "-o", dest, src)
		out, err := cmd.CombinedOutput()
		if err != nil {
			acpMockErr = fmt.Errorf("build acpmock: %w\n%s", err, out)
			return
		}
		acpMockPath = dest
	})
	if acpMockErr != nil {
		tb.Fatal(acpMockErr)
	}
	return acpMockPath
}

func TestModel_GenerateContent_Success(t *testing.T) {
	mockPath := buildACPMock(t)

	m := NewModel(Config{
		APIKey:   "test-key",
		Model:    "test-model",
		BasePath: mockPath,
	})

	req := &model.LLMRequest{
		Model:    "test-model",
		Contents: genai.Text("generate a commit message"),
		Config:   &genai.GenerateContentConfig{},
	}

	var response *model.LLMResponse
	for resp := range m.GenerateContent(context.Background(), req, false) {
		if resp != nil && resp.Content != nil {
			response = resp
		}
	}

	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Content == nil {
		t.Fatal("expected non-nil content")
	}

	var text string
	for _, part := range response.Content.Parts {
		if part != nil {
			text += part.Text
		}
	}
	if !strings.Contains(text, "Generated commit message") {
		t.Errorf("expected text to contain 'Generated commit message', got %q", text)
	}
}

func TestModel_GenerateContent_BinaryNotFound(t *testing.T) {
	m := NewModel(Config{
		APIKey:   "test-key",
		BasePath: "/nonexistent/acp-binary",
	})

	req := &model.LLMRequest{
		Model:    "test-model",
		Contents: genai.Text("test"),
		Config:   &genai.GenerateContentConfig{},
	}

	hadError := false
	for range m.GenerateContent(context.Background(), req, false) {
		hadError = true
	}
	if !hadError {
		t.Error("expected error for nonexistent binary, got no error")
	}
}

// Test that the Go JSON serialization of our RPC messages works correctly
func TestJSONRPCMessageSerialization(t *testing.T) {
	id := 1
	msg := jsonrpcMessage{
		Jsonrpc: "2.0",
		ID:      &id,
		Method:  "session/prompt",
		Params: mustMarshal(map[string]interface{}{
			"sessionId": "sess_1",
			"prompt": []map[string]interface{}{
				{"type": "text", "text": "hello"},
			},
		}),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded jsonrpcMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Jsonrpc != "2.0" {
		t.Errorf("Jsonrpc = %q, want %q", decoded.Jsonrpc, "2.0")
	}
	if decoded.ID == nil || *decoded.ID != 1 {
		t.Errorf("ID = %v, want 1", decoded.ID)
	}
	if decoded.Method != "session/prompt" {
		t.Errorf("Method = %q, want %q", decoded.Method, "session/prompt")
	}
}

func TestMustMarshal(t *testing.T) {
	t.Parallel()
	result := mustMarshal(map[string]string{"key": "value"})
	var decoded map[string]string
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded["key"] != "value" {
		t.Errorf("key = %q, want %q", decoded["key"], "value")
	}
}

// Test that the Model can be reused across multiple GenerateContent calls
// using the same client/session (keep-alive).
func TestModel_GenerateContent_KeepAlive(t *testing.T) {
	mockPath := buildACPMock(t)

	m := NewModel(Config{
		APIKey:   "test-key",
		BasePath: mockPath,
	})

	// First call
	req := &model.LLMRequest{
		Model:    "test-model",
		Contents: genai.Text("first prompt"),
		Config:   &genai.GenerateContentConfig{},
	}

	respText := func(resp *model.LLMResponse) string {
		var text string
		if resp != nil && resp.Content != nil {
			for _, part := range resp.Content.Parts {
				if part != nil {
					text += part.Text
				}
			}
		}
		return text
	}

	for resp := range m.GenerateContent(context.Background(), req, false) {
		if resp != nil && resp.Content != nil {
			if got := respText(resp); got != "Generated commit message #1" {
				t.Errorf("first call: got %q, want %q", got, "Generated commit message #1")
			}
		}
	}

	// Second call — should reuse the same client + session (keep-alive).
	// The mock stays alive and processes the second prompt in the same session.
	req2 := &model.LLMRequest{
		Model:    "test-model",
		Contents: genai.Text("second prompt"),
		Config:   &genai.GenerateContentConfig{},
	}

	for resp := range m.GenerateContent(context.Background(), req2, false) {
		if resp != nil && resp.Content != nil {
			if got := respText(resp); got != "Generated commit message #2" {
				t.Errorf("second call: got %q, want %q", got, "Generated commit message #2")
			}
		}
	}

	// Also verify the client is still alive after two prompts.
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()

	if client == nil {
		t.Fatal("expected client to still be alive after two calls")
	}
	if !client.alive() {
		t.Error("expected client to be alive after two prompts (keep-alive)")
	}
	if client.sessionID != "sess_mock_001" {
		t.Errorf("expected same session across calls, got sessionID = %q", client.sessionID)
	}
}

// Test that the Model reuses the same client when alive (keep-alive).
// This tests that getOrStartClient returns the existing live client
// instead of creating a new one.
func TestModel_GenerateContent_ReusesClient(t *testing.T) {
	mockPath := buildACPMock(t)

	m := NewModel(Config{
		APIKey:   "test-key",
		BasePath: mockPath,
	})

	req := &model.LLMRequest{
		Model:    "test-model",
		Contents: genai.Text("prompt one"),
		Config:   &genai.GenerateContentConfig{},
	}

	// First call — creates the client
	for range m.GenerateContent(context.Background(), req, false) {
	}

	// Capture the client after the first call
	m.mu.Lock()
	clientAfterFirst := m.client
	m.mu.Unlock()

	if clientAfterFirst == nil {
		t.Fatal("expected client to be set after first call")
	}
	if !clientAfterFirst.alive() {
		t.Fatal("expected client to be alive for second call")
	}

	// Second call — should reuse the same client
	for range m.GenerateContent(context.Background(), req, false) {
	}

	m.mu.Lock()
	clientAfterSecond := m.client
	m.mu.Unlock()

	// The client pointer should be the same (reused, not recreated)
	if clientAfterSecond != clientAfterFirst {
		t.Error("expected second call to reuse the same client instance (keep-alive)")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation tests
// ---------------------------------------------------------------------------

// newBlockingClient creates an acpClient whose decoder blocks on reads.
// The caller controls when data arrives via the returned write end of the
// pipe. Drain done when shutting down.
func newBlockingClient(t *testing.T) (*acpClient, *io.PipeWriter, *io.PipeReader) {
	t.Helper()

	// Pipe for the client's decoder (stdout from subprocess)
	stdoutR, stdoutW := io.Pipe()

	// Pipe for the client's stdin (stdin to subprocess)
	stdinR, stdinW := io.Pipe()

	c := &acpClient{
		stdin:   stdinW,
		decoder: json.NewDecoder(stdoutR),
		nextID:  0,
		done:    make(chan struct{}),
	}

	return c, stdoutW, stdinR
}

func TestReadResponse_ContextCancellation(t *testing.T) {
	t.Parallel()
	c, stdoutW, _ := newBlockingClient(t)
	defer stdoutW.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		_, err := c.readResponse(ctx, 1)
		errCh <- err
	}()

	// Give the goroutine time to enter the decode loop and block
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: readResponse did not return after cancellation")
	}
}

func TestSendPrompt_ContextCancellation(t *testing.T) {
	t.Parallel()
	c, stdoutW, stdinR := newBlockingClient(t)
	defer stdoutW.Close()
	defer stdinR.Close()

	c.sessionID = "test-session"

	// Drain the request that sendPrompt writes to stdin so sendRequest
	// doesn't block (pipe buffer can fill up).
	reqRead := make(chan struct{}, 1)
	go func() {
		var msg jsonrpcMessage
		json.NewDecoder(stdinR).Decode(&msg)
		reqRead <- struct{}{}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		_, err := c.sendPrompt(ctx, &model.LLMRequest{
			Contents: genai.Text("test prompt"),
		})
		errCh <- err
	}()

	// Wait for the request to be consumed, then let sendPrompt enter its
	// decode loop.
	<-reqRead
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: sendPrompt did not return after cancellation")
	}
}

// ---------------------------------------------------------------------------
// Goroutine leak tests
// ---------------------------------------------------------------------------

// awaitGoroutineCount polls until the goroutine count drops to at most the
// expected count, or until the deadline expires and calls t.Errorf.
func awaitGoroutineCount(t *testing.T, initial int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if runtime.NumGoroutine() <= initial {
			return
		}
		select {
		case <-deadline:
			t.Errorf("goroutine leak: started with %d goroutines, ended with %d",
				initial, runtime.NumGoroutine())
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestReadResponse_GoroutineLeak(t *testing.T) {
	t.Parallel()

	initial := runtime.NumGoroutine()

	c, stdoutW, _ := newBlockingClient(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := c.readResponse(ctx, 1)
		errCh <- err
	}()

	// Let the read goroutine enter the decode loop and block on Decode.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: readResponse did not return after cancellation")
	}

	// Close the pipe writer so the decode goroutine unblocks (receives EOF),
	// completes the Decode call, and can exit via the <-ctx.Done() path.
	stdoutW.Close()

	awaitGoroutineCount(t, initial)
}

func TestSendPrompt_GoroutineLeak(t *testing.T) {
	t.Parallel()

	initial := runtime.NumGoroutine()

	c, stdoutW, stdinR := newBlockingClient(t)
	c.sessionID = "test-session"

	// Drain the request that sendPrompt writes to stdin so sendRequest
	// doesn't block (pipe buffer can fill up).
	reqRead := make(chan struct{}, 1)
	go func() {
		var msg jsonrpcMessage
		_ = json.NewDecoder(stdinR).Decode(&msg)
		reqRead <- struct{}{}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := c.sendPrompt(ctx, &model.LLMRequest{
			Contents: genai.Text("test prompt"),
		})
		errCh <- err
	}()

	// Wait for the request to be consumed, then let sendPrompt enter its
	// decode loop.
	<-reqRead
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: sendPrompt did not return after cancellation")
	}

	// Close the pipe writer so the decode goroutine unblocks and exits.
	stdoutW.Close()
	stdinR.Close()

	awaitGoroutineCount(t, initial)
}

// ---------------------------------------------------------------------------
// Oracle tests: model forwarding to ACP subprocess
// ---------------------------------------------------------------------------

// pipePair creates a connected pipe pair for intercepting JSON-RPC writes.
func pipePair(t *testing.T) (*io.PipeWriter, *io.PipeReader) {
	t.Helper()
	r, w := io.Pipe()
	return w, r
}

// captureACPMessageFromSendPrompt creates a client with piped stdin,
// calls sendPrompt in a goroutine, and returns the JSON-RPC message
// written to the pipe. The caller must close stdoutW after the prompt
// returns to avoid goroutine leaks.
func captureACPMessageFromSendPrompt(
	t *testing.T, reqModel string,
) (*jsonrpcMessage, *io.PipeWriter, context.CancelFunc) {
	t.Helper()

	c, stdoutW, stdinR := newBlockingClient(t)
	c.sessionID = "oracle-test-session"

	req := &model.LLMRequest{
		Model:    reqModel,
		Contents: genai.Text("test prompt"),
		Config:   &genai.GenerateContentConfig{},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Read the JSON-RPC request from the pipe
	msgCh := make(chan *jsonrpcMessage, 1)
	go func() {
		var msg jsonrpcMessage
		if err := json.NewDecoder(stdinR).Decode(&msg); err != nil {
			t.Logf("decode stdin: %v", err)
		}
		msgCh <- &msg
	}()

	// sendPrompt writes the request, then blocks waiting for a response.
	// We cancel the context after capturing the message so it returns.
	go func() {
		_, _ = c.sendPrompt(ctx, req)
	}()

	return <-msgCh, stdoutW, cancel
}

// Test that req.Model is forwarded faithfully in the session/prompt params.
// Regression guard: if sendPrompt ever drops req.Model, this test catches it.
func TestOracle_SendPrompt_ForwardsModel(t *testing.T) {
	t.Parallel()

	msg, stdoutW, cancel := captureACPMessageFromSendPrompt(t, "test-model-42")
	defer stdoutW.Close()
	cancel()

	var params map[string]interface{}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	model, ok := params["model"]
	if !ok {
		t.Fatal("session/prompt params missing 'model' key — req.Model is not forwarded")
	}
	if model != "test-model-42" {
		t.Errorf("params['model'] = %v, want 'test-model-42'", model)
	}
}

// Test that forward slash characters in model paths survive JSON serialization.
func TestOracle_SendPrompt_ForwardsModelWithSlash(t *testing.T) {
	t.Parallel()

	msg, stdoutW, cancel := captureACPMessageFromSendPrompt(t, "org/model-name")
	defer stdoutW.Close()
	cancel()

	var params map[string]interface{}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["model"] != "org/model-name" {
		t.Errorf("params['model'] = %v, want 'org/model-name'", params["model"])
	}
}

// Test that the model field survives JSON round-trip and appears alongside
// the sessionId and prompt fields in the same params object.
func TestOracle_SendPrompt_ModelCoexistsWithOtherParams(t *testing.T) {
	t.Parallel()

	msg, stdoutW, cancel := captureACPMessageFromSendPrompt(t, "my-model")
	defer stdoutW.Close()
	cancel()

	var params map[string]interface{}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if params["sessionId"] != "oracle-test-session" {
		t.Errorf("params['sessionId'] = %v, want 'oracle-test-session'", params["sessionId"])
	}
	if params["model"] != "my-model" {
		t.Errorf("params['model'] = %v, want 'my-model'", params["model"])
	}
	if _, ok := params["prompt"]; !ok {
		t.Error("params missing 'prompt' key")
	}
}

// Test that the config stores the API key correctly.
func TestConfigStoresAPIKey(t *testing.T) {
	m := NewModel(Config{
		APIKey:   "my-secret-key",
		BasePath: "opencode",
	})

	if m.config.APIKey != "my-secret-key" {
		t.Errorf("config.APIKey = %q, want %q", m.config.APIKey, "my-secret-key")
	}
}
