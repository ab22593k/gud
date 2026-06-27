package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

// acpClient manages the lifecycle of an opencode acp subprocess and
// provides JSON-RPC communication over its stdio pipes.
type acpClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	decoder   *json.Decoder
	nextID    int
	sessionID string
	decodeCh  chan decodeResult
	done      chan struct{} // closed by Wait() goroutine when process exits
	closeOnce sync.Once
}

// decodeResult carries a decoded JSON-RPC message or error from the
// client's background decode loop.
type decodeResult struct {
	msg jsonrpcMessage
	err error
}

// startACPClient spawns `opencode acp`, initializes the ACP connection,
// and creates a new session.
func startACPClient(ctx context.Context, cfg Config) (*acpClient, error) {
	//nolint:gosec // cfg.BasePath is a config default, not arbitrary user input
	cmd := exec.CommandContext(ctx, cfg.BasePath, "acp")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()

		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	cmd.Env = os.Environ()
	if cfg.APIKey != "" {
		cmd.Env = append(cmd.Env, "OPENCODE_API_KEY="+cfg.APIKey)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()

		return nil, fmt.Errorf("failed to start %q acp: %w (is opencode installed?)", cfg.BasePath, err)
	}

	c := &acpClient{
		cmd:      cmd,
		stdin:    stdin,
		decoder:  json.NewDecoder(stdout),
		decodeCh: make(chan decodeResult, 1),
		done:     make(chan struct{}),
	}

	// Monitor process exit in a goroutine so alive() works correctly.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("opencode acp exited with error", "error", err)
		}
		close(c.done)
	}()

	// Start a single background decode goroutine. It runs for the entire
	// lifetime of this client, eliminating goroutine leaks on context
	// cancellation.
	go c.decodeLoop()

	if err := c.initialize(ctx); err != nil {
		c.close()

		return nil, err
	}
	if err := c.createSession(ctx); err != nil {
		c.close()

		return nil, err
	}

	return c, nil
}

// decodeLoop runs in a background goroutine, continuously decoding
// JSON-RPC messages from stdout and forwarding them to decodeCh.
// It exits when Decode fails (pipe closed) or c.done is signalled.
func (c *acpClient) decodeLoop() {
	defer close(c.decodeCh)

	for {
		var msg jsonrpcMessage
		err := c.decoder.Decode(&msg)
		select {
		case c.decodeCh <- decodeResult{msg, err}:
		case <-c.done:
			return
		}
		if err != nil {
			return
		}
	}
}

// alive reports whether the subprocess is still running.
func (c *acpClient) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// close terminates the subprocess and blocks until it has exited.
func (c *acpClient) close() {
	c.closeOnce.Do(func() {
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		// Wait for the Wait goroutine to finish, preventing zombies.
		<-c.done
	})
}

// sendRequest encodes and writes a JSON-RPC request to the subprocess stdin.
// Returns the request ID so the caller can match the response.
func (c *acpClient) sendRequest(method string, params interface{}) (int, error) {
	c.nextID++
	id := c.nextID

	msg := jsonrpcMessage{
		Jsonrpc: jsonrpcVersion,
		ID:      &id,
		Method:  method,
		Params:  mustMarshal(params),
	}

	if err := json.NewEncoder(c.stdin).Encode(msg); err != nil {
		return 0, fmt.Errorf("failed to send %s request: %w", method, err)
	}

	return id, nil
}

// sendNotification writes a JSON-RPC notification (no ID, no response expected).
func (c *acpClient) sendNotification(method string, params interface{}) error {
	msg := jsonrpcMessage{
		Jsonrpc: jsonrpcVersion,
		Method:  method,
		Params:  mustMarshal(params),
	}
	if err := json.NewEncoder(c.stdin).Encode(msg); err != nil {
		return fmt.Errorf("failed to send %s notification: %w", method, err)
	}

	return nil
}

// readResponse reads JSON-RPC messages from the decode loop channel until a
// response with the expected ID arrives. Notifications (messages without an
// ID) are silently skipped. It respects context cancellation.
func (c *acpClient) readResponse(ctx context.Context, expectedID int) (json.RawMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r, ok := <-c.decodeCh:
			if !ok {
				return nil, errors.New("acp client: connection closed")
			}
			if r.err != nil {
				return nil, fmt.Errorf("failed to read response: %w", r.err)
			}

			if r.msg.ID == nil {
				continue
			}

			if *r.msg.ID != expectedID {
				continue
			}

			if r.msg.Error != nil {
				return nil, fmt.Errorf("RPC error %d: %s", r.msg.Error.Code, r.msg.Error.Message)
			}

			return r.msg.Result, nil
		}
	}
}

// doRequest is a request/response helper that skips notifications.
// It respects context cancellation.
func (c *acpClient) doRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id, err := c.sendRequest(method, params)
	if err != nil {
		return nil, err
	}

	return c.readResponse(ctx, id)
}
