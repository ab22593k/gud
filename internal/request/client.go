package request

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

// ContentResponse represents a response from content generation.
type ContentResponse interface {
	Text() string
}

// ClientConfig holds configuration for creating a Client.
// Temperature is intentionally omitted — deprecated by Google for Gemini 3.6+.
type ClientConfig struct {
	APIKey string
	Model  string
	// EmbeddingModel is the Gemini model used to embed diffs for vector
	// retrieval. When empty, defaultEmbeddingModel is used.
	EmbeddingModel string
}

// Client wraps an ADK model.LLM for generating commit messages and, when
// available, a Gemini client for text embeddings (hybrid memory retrieval).
type Client struct {
	modelImpl      model.LLM
	model          string
	embeddingModel string
	embedFn        func(ctx context.Context, text string) ([]float32, error)
}

const defaultModel = "gemini-flash-lite-latest"

// NewClient creates a new request client.
// The caller is responsible for providing a context that can carry timeouts
// and cancellation.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
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

	embeddingModel := cfg.EmbeddingModel
	if embeddingModel == "" {
		embeddingModel = defaultEmbeddingModel
	}

	// A separate genai client powers embeddings; the ADK model handles content
	// generation. Sharing the API key keeps a single credential in config.
	gClient, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.APIKey})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	slog.Debug("created gemini client via ADK", "model", cfg.Model, "embeddingModel", embeddingModel)

	c := &Client{
		modelImpl:      adkModel,
		model:          cfg.Model,
		embeddingModel: embeddingModel,
	}
	c.embedFn = func(ctx context.Context, text string) ([]float32, error) {
		resp, err := gClient.Models.EmbedContent(ctx, embeddingModel,
			genai.Text(text), &genai.EmbedContentConfig{TaskType: "RETRIEVAL_DOCUMENT"})
		if err != nil {
			return nil, fmt.Errorf("embed content: %w", err)
		}
		if len(resp.Embeddings) == 0 || resp.Embeddings[0] == nil {
			return nil, fmt.Errorf("embed content: empty response")
		}
		return resp.Embeddings[0].Values, nil
	}

	return c, nil
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

// GenerateCommitMessage generates a commit message based on the provided diff.
func (c *Client) GenerateCommitMessage(ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string, profile ProfileName) (string, error) {
	return c.GenerateCommitMessageWithContent(ctx, diff, commitContext, detailLevel, hint, profile, "", defaultWrapLine)
}

// GenerateCommitMessageWithContent generates a commit message with an optional custom system prompt content.
func (c *Client) GenerateCommitMessageWithContent(ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string, profile ProfileName, systemContent string, wrapLine int) (string, error) {
	if diff == "" {
		return "", fmt.Errorf("diff is required")
	}

	slog.Debug("generating commit message", "model", c.model, "detailLevel", detailLevel)

	prompt := BuildCommitMessagePromptWithContent(diff, commitContext, detailLevel, hint, profile, systemContent, wrapLine)

	req := &model.LLMRequest{
		Model:    c.model,
		Contents: genai.Text(prompt),
		Config:   &genai.GenerateContentConfig{},
	}

	result, err := generateContent(c, ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if result == "" {
		return "", fmt.Errorf("generated message is empty")
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
			return "", fmt.Errorf("model returned nil response")
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
		return "", fmt.Errorf("no response from model")
	}

	return extractText(response.Content)
}

// extractText pulls text from genai.Content parts and then sanitizes it.
func extractText(content *genai.Content) (string, error) {
	if content == nil {
		return "", fmt.Errorf("content is nil")
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

func stripAfterDelimiter(text string) string {
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || trimmed == "___" || trimmed == "***" {
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		}
	}

	return text
}
