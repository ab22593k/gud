package request

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
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
	// defaultModel is the Gemini model used when none is configured.
	defaultModel = "gemini-flash-lite-latest"

	// defaultGenerateTimeout bounds a single content-generation call when
	// the caller's context carries no deadline. Without it, a hung API
	// would stall the CLI (and a prepare-commit-msg hook) indefinitely.
	defaultGenerateTimeout = 2 * time.Minute
)

// NewClient creates a new request client.
// The caller is responsible for providing a context that can carry timeouts
// and cancellation.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}

	return newGeminiClient(ctx, cfg)
}

// newGeminiClient creates a client using the ADK Gemini model.
func newGeminiClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	adkModel, err := gemini.NewModel(ctx, cfg.Model, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini model: %w", err)
	}

	slog.Debug("created gemini client via ADK", "model", cfg.Model)

	return &Client{
		modelImpl: adkModel,
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
	ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string, profile ProfileName,
) (string, error) {
	return c.GenerateCommitMessageWithContent(ctx, diff, commitContext, detailLevel, hint, profile, "", defaultWrapLine)
}

// GenerateCommitMessageWithContent generates a commit message with an optional custom system prompt content.
func (c *Client) GenerateCommitMessageWithContent(
	ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string, profile ProfileName,
	systemContent string, wrapLine int,
) (string, error) {
	if diff == "" {
		return "", errors.New("diff is required")
	}

	slog.Debug("generating commit message", "model", c.model, "detailLevel", detailLevel)

	prompt := BuildCommitMessagePromptWithContent(diff, commitContext, detailLevel, hint, profile, systemContent, wrapLine)

	req := &model.LLMRequest{
		Model:    c.model,
		Contents: genai.Text(prompt),
		Config:   &genai.GenerateContentConfig{},
	}

	ctx, cancel := withDefaultTimeout(ctx, defaultGenerateTimeout)
	defer cancel()

	result, err := generateContent(c, ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if result == "" {
		return "", errors.New("generated message is empty")
	}

	return result, nil
}

func generateContent(c *Client, ctx context.Context, req *model.LLMRequest) (string, error) {
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

// sanitizeOutput cleans AI-generated text.
func sanitizeOutput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}

	text = extractFromCodeFences(text)
	text = stripPreamble(text)
	text = stripAfterDelimiter(text)

	return strings.TrimSpace(text)
}

func extractFromCodeFences(text string) string {
	lines := strings.Split(text, "\n")

	if len(lines) > 1 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		for i := len(lines) - 1; i > 0; i-- {
			if strings.TrimSpace(lines[i]) == "```" {
				return strings.Join(lines[1:i], "\n")
			}
		}

		return strings.Join(lines[1:], "\n")
	}

	if strings.HasPrefix(text, "`") && strings.HasSuffix(text, "`") {
		inner := text[1 : len(text)-1]
		if !strings.Contains(inner, "`") {
			return inner
		}
	}

	return text
}

func stripPreamble(text string) string {
	preamblePrefixes := []string{
		"here is your commit message",
		"here's the commit message",
		"here is the generated commit message",
		"generated commit message",
		"here is the commit",
		"here's your commit",
		"commit message:",
		"the commit message:",
		"generated message:",
		"output:",
		"result:",
	}

	for {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) < 2 {
			return text
		}

		firstLine := strings.TrimSpace(lines[0])
		firstLower := strings.ToLower(firstLine)

		matched := false
		for _, prefix := range preamblePrefixes {
			if strings.HasPrefix(firstLower, prefix) {
				text = strings.TrimSpace(lines[1])
				matched = true

				break
			}
		}
		if !matched {
			break
		}
	}

	return text
}

// stripAfterDelimiter removes AI trailing commentary that follows a separator
// line (---, ___, ***). It scans from the end and only cuts at a separator
// when the trailing content looks like short commentary; a legitimate commit
// body that happens to contain such a separator line is preserved.
func stripAfterDelimiter(text string) string {
	lines := strings.Split(text, "\n")

	for i := range slices.Backward(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "---" && trimmed != "___" && trimmed != "***" {
			continue
		}

		// Only treat this separator as a delimiter if everything after it is
		// short commentary. Substantive body text after a --- line is kept.
		if isShortCommentaryTail(lines[i+1:]) {
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		}
	}

	return text
}

// maxCommentaryLines and maxCommentaryTotal bound what stripAfterDelimiter
// considers AI trailing commentary rather than a commit body. AI commentary
// after a separator is at most a couple of short lines ("Generated by AI");
// anything longer — more lines or a body-sized block — is treated as commit
// content and preserved. Blank lines are allowed: a separator followed by a
// blank line then a short note ("---\n\nGenerated by AI") is still
// commentary, while substantive bodies exceed the length bound.
const (
	maxCommentaryLines = 2
	maxCommentaryTotal = 60
)

// isShortCommentaryTail reports whether the lines after a separator look like
// AI trailing commentary: at most a couple of lines totalling a small amount
// of text. A longer or multi-line block is treated as commit body content.
func isShortCommentaryTail(lines []string) bool {
	nonBlank, total := 0, 0
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		nonBlank++
		total += len(line)
		if nonBlank > maxCommentaryLines {
			return false
		}
	}

	return total <= maxCommentaryTotal
}
