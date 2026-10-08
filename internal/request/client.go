// Package request implements LLM-backed commit message generation on top of
// the Gemini model: prompt construction, streaming generation, and response
// post-processing.
package request

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gud/internal/obs"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// ContentResponse represents a response from content generation.
type ContentResponse interface {
	Text() string
}

// ClientConfig holds configuration for creating a Client.
type ClientConfig struct {
	APIKey string
	Model  string
}

// Client wraps an ADK model.LLM for generating commit messages.
type Client struct {
	modelImpl model.LLM
	model     string
}

const (
	// defaultModel is the Gemini model the Antigravity agent reasons with when
	// none is configured: the agent's cheapest, lowest-latency option, which is
	// all a commit message needs.
	defaultModel = "gemini-3.5-flash-lite"

	// defaultGenerateTimeout bounds a single content-generation call when
	// the caller's context carries no deadline. Without it, a hung API
	// would stall the CLI (and a prepare-commit-msg hook) indefinitely.
	//
	// The Antigravity agent provisions a sandbox and runs a tool-use loop
	// before it can answer, so a call takes minutes rather than the seconds a
	// plain model call needs; the documented client-side timeout for it is 300s.
	// https://ai.google.dev/gemini-api/docs/antigravity-agent
	defaultGenerateTimeout = 5 * time.Minute
)

// NewClient creates a new request client.
// The caller is responsible for providing a context that can carry timeouts
// and cancellation.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("API key is required")
	}

	// The agent serves a fixed model set, so the configured name is resolved to
	// an available one here. Client.model — and with it ModelName() and the
	// Assisted-by trailer — then names the model that actually served the
	// request instead of the one that was asked for.
	configured := cfg.Model
	if configured == "" {
		configured = defaultModel
	}

	model := antigravityModel(configured)
	if model != configured {
		slog.Debug("configured model is not offered by the Antigravity agent; using its default",
			"configured", configured, "model", model)
	}

	cfg.Model = model

	return newGeminiClient(ctx, cfg)
}

// newGeminiClient creates a client backed by the Gemini Antigravity agent.
func newGeminiClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	slog.Debug("created gemini client via the antigravity agent",
		"agent", antigravityAgent, "model", cfg.Model)

	return &Client{
		modelImpl: newInteractionsModel(cfg.Model, genaiClient.Interactions),
		model:     cfg.Model,
	}, nil
}

// ModelName returns the model name used by this client.
func (c *Client) ModelName() string {
	return c.model
}

// NewClientWithGenerator creates a new client with a custom model for testing.
func NewClientWithGenerator(llm model.LLM, modelName string) *Client {
	if modelName == "" {
		modelName = defaultModel
	}

	return &Client{
		modelImpl: llm,
		model:     modelName,
	}
}

// withDefaultTimeout returns a context with the given timeout if the caller's
// context has no deadline. It preserves explicit caller deadlines so a
// hook-mode or user-visible cancellation is never overridden by the default.
func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, d)
}

// GenerateCommitMessage generates a commit message based on the provided diff.
func (c *Client) GenerateCommitMessage(
	ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string,
) (string, error) {
	return c.GenerateCommitMessageWithContent(ctx, diff, commitContext, detailLevel, hint, "", defaultWrapLine)
}

// GenerateCommitMessageWithContent generates a commit message, mounting
// systemContent as AGENTS.md in the agent's environment. An empty
// systemContent leaves the default system prompt inline in the task.
func (c *Client) GenerateCommitMessageWithContent(
	ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string,
	systemContent string, wrapLine int,
) (string, error) {
	if diff == "" {
		return "", errors.New("diff is required")
	}

	instructions, task := BuildAgentPrompt(diff, commitContext, detailLevel, hint, systemContent, wrapLine)

	slog.Debug("generating commit message", "model", c.model, "detailLevel", detailLevel, "diff_bytes", len(diff),
		"ctx_bytes", len(commitContext), "agents_md_bytes", len(instructions))

	// The instructions ride on the request rather than inside the prompt so the
	// adapter can mount them as AGENTS.md. Leaving them inline as well would
	// send the same guidance twice, by two different routes.
	cfg := &genai.GenerateContentConfig{}
	if instructions != "" {
		cfg.SystemInstruction = genai.NewContentFromText(instructions, "system")
	}

	req := &model.LLMRequest{
		Model:    c.model,
		Contents: genai.Text(task),
		Config:   cfg,
	}

	ctx, cancel := withDefaultTimeout(ctx, defaultGenerateTimeout)
	defer cancel()

	tm := obs.Start("model.generate")

	result, err := generateVerified(ctx, c, req, diff, wrapLine)

	tm.Done("model", c.model, "ok", err == nil)

	if err != nil {
		return "", err
	}

	return result, nil
}

// maxVerifyAttempts bounds verify-then-regenerate: 1 initial generation plus
// at most 1 regeneration. Heuristic findings never fail the call — the last
// output is always served — so this costs at most one extra call.
const maxVerifyAttempts = 2

// generateVerified generates a message and checks it against the diff,
// regenerating when verification fails. Transport errors and empty outputs
// return immediately with the historical error messages; persistent heuristic
// findings serve the last output.
func generateVerified(
	ctx context.Context, c *Client, req *model.LLMRequest, diff string, wrapLine int,
) (string, error) {
	var result string

	for attempt := 1; attempt <= maxVerifyAttempts; attempt++ {
		var err error

		result, err = generateWithRetry(ctx, c, req)
		if err != nil {
			return "", fmt.Errorf("failed to generate content: %w", err)
		}

		if result == "" {
			return "", errors.New("generated message is empty")
		}

		findings := VerifyMessage(result, diff, wrapLine)
		if len(findings) == 0 {
			return result, nil
		}

		slog.Debug("message verification failed, regenerating",
			"attempt", attempt, "findings", findingCodes(findings))

		if attempt == maxVerifyAttempts {
			slog.Debug("message verification still failing, serving last output",
				"findings", findingCodes(findings))
		}
	}

	return result, nil
}

// isTransientErr reports whether err looks retryable: timeouts, 429, 5xx,
// or unavailable/connection-reset phrases. Auth errors (401/403) and empty
// responses are not transient.
func isTransientErr(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"timeout", "deadline exceeded", "unavailable", "connection reset",
		"connection refused", "429", "500", "502", "503", "504",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}

	return false
}

// generateWithRetry calls generateContent with up to 2 retries on transient
// errors (200ms then 500ms backoff). It respects ctx cancellation.
func generateWithRetry(ctx context.Context, c *Client, req *model.LLMRequest) (string, error) {
	backoffs := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond}

	var result string

	var err error

	for attempt := 0; ; attempt++ {
		result, err = generateContent(ctx, c, req)
		if err == nil || ctx.Err() != nil || !isTransientErr(err) || attempt >= len(backoffs) {
			return result, err
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoffs[attempt]):
			slog.Debug("retrying model call", "attempt", attempt+1, "error", err)
		}
	}
}

func generateContent(ctx context.Context, c *Client, req *model.LLMRequest) (string, error) {
	var response *model.LLMResponse

	for resp, err := range c.modelImpl.GenerateContent(ctx, req, false) {
		if err != nil {
			return "", fmt.Errorf("model error: %w", err)
		}

		if resp == nil {
			return "", errors.New("model returned nil response")
		}

		if resp.ErrorMessage != "" {
			return "", fmt.Errorf("model error: %s", resp.ErrorMessage)
		}

		if resp.ErrorCode != "" {
			return "", fmt.Errorf("model error code: %s", resp.ErrorCode)
		}

		response = resp
	}

	if response == nil {
		return "", errors.New("no response from model")
	}

	return extractText(response.Content)
}

// extractText pulls text from genai.Content parts and then sanitizes it.
func extractText(content *genai.Content) (string, error) {
	if content == nil {
		return "", errors.New("content is nil")
	}

	var sb strings.Builder

	for _, part := range content.Parts {
		if part == nil {
			continue
		}

		sb.WriteString(part.Text)
	}

	return sanitizeOutput(sb.String()), nil
}
