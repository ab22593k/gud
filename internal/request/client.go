package request

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gud/internal/opencode"

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
	APIKey      string
	Model       string
	Temperature float64
	ACP         string // "gemini" or "opencode"
}

// Client wraps an ADK model.LLM for generating commit messages.
type Client struct {
	modelImpl   model.LLM
	model       string
	temperature *float32
}

const defaultModel = "gemini-3.1-flash-lite"

// NewClient creates a new request client.
// The caller is responsible for providing a context that can carry timeouts
// and cancellation.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.APIKey == "" && cfg.ACP != "opencode" {
		return nil, fmt.Errorf("API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}

	switch cfg.ACP {
	case "opencode":
		return newOpenCodeClient(ctx, cfg)
	default:
		return newGeminiClient(ctx, cfg)
	}
}

// newGeminiClient creates a client using the ADK Gemini model.
func newGeminiClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	adkModel, err := gemini.NewModel(ctx, cfg.Model, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini model: %w", err)
	}

	var temp *float32
	if cfg.Temperature != 0 {
		t := float32(cfg.Temperature)
		temp = &t
	}

	slog.Debug("created gemini client via ADK", "model", cfg.Model, "temperature", cfg.Temperature)

	return &Client{
		modelImpl:   adkModel,
		model:       cfg.Model,
		temperature: temp,
	}, nil
}

// newOpenCodeClient creates a client using the OpenCode.ai provider.
func newOpenCodeClient(_ context.Context, cfg ClientConfig) (*Client, error) {
	modelName := cfg.Model
	if modelName == "" || modelName == defaultModel {
		modelName = "deepseek-v4-flash"
	}

	openCodeModel := opencode.NewModel(opencode.Config{
		APIKey: cfg.APIKey,
		Model:  modelName,
	})

	var temp *float32
	if cfg.Temperature != 0 {
		t := float32(cfg.Temperature)
		temp = &t
	}

	slog.Debug("created opencode client", "model", modelName, "temperature", cfg.Temperature)

	return &Client{
		modelImpl:   openCodeModel,
		model:       modelName,
		temperature: temp,
	}, nil
}

// ModelName returns the model name used by this client.
func (c *Client) ModelName() string {
	return c.model
}

// NewClientWithGenerator creates a new client with a custom model for testing.
func NewClientWithGenerator(llm model.LLM, modelName string, temperature float64) *Client {
	if modelName == "" {
		modelName = defaultModel
	}
	var temp *float32
	if temperature != 0 {
		t := float32(temperature)
		temp = &t
	}
	return &Client{
		modelImpl:   llm,
		model:       modelName,
		temperature: temp,
	}
}

// GenerateCommitMessage generates a commit message based on the provided diff, commit context, detail level, hint, and persona.
func (c *Client) GenerateCommitMessage(ctx context.Context, diff, commitContext string, detailLevel DetailLevel, hint string, persona PersonaName) (string, error) {
	if diff == "" {
		return "", fmt.Errorf("diff is required")
	}

	slog.Debug("generating commit message", "model", c.model, "detailLevel", detailLevel)

	prompt := BuildCommitMessagePrompt(diff, commitContext, detailLevel, hint, persona)

	req := &model.LLMRequest{
		Model:    c.model,
		Contents: genai.Text(prompt),
		Config: &genai.GenerateContentConfig{
			Temperature: c.temperature,
		},
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

// sanitizeOutput cleans AI-generated text by stripping markdown code fences,
// common preamble text, and trailing AI commentary. It also trims whitespace.
// The goal is to enforce plain-text output suitable for direct use (e.g., git commit messages).
func sanitizeOutput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}

	// Step 1: Extract content from markdown code fences if the whole response is wrapped
	text = extractFromCodeFences(text)

	// Step 2: Strip common preamble lines ("Here is your commit message:", etc.)
	text = stripPreamble(text)

	// Step 3: Truncate at common trailing delimiters
	text = stripAfterDelimiter(text)

	// Step 4: Final trim
	return strings.TrimSpace(text)
}

// extractFromCodeFences checks if the text is wrapped in markdown code fences
// (triple backticks) and extracts the content from inside.
func extractFromCodeFences(text string) string {
	lines := strings.Split(text, "\n")

	// Check if first line is a code fence opener (``` or ```language)
	if len(lines) > 1 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		// Find closing fence
		for i := len(lines) - 1; i > 0; i-- {
			if strings.TrimSpace(lines[i]) == "```" {
				return strings.Join(lines[1:i], "\n")
			}
		}
		// No closing fence found; remove the opening fence line
		return strings.Join(lines[1:], "\n")
	}

	// Check for inline backtick wrapping (single backticks around whole message)
	if strings.HasPrefix(text, "`") && strings.HasSuffix(text, "`") {
		inner := text[1 : len(text)-1]
		// Only strip if there's no unclosed backtick inside
		if !strings.Contains(inner, "`") {
			return inner
		}
	}

	return text
}

// stripPreamble iteratively removes common introductory lines that models add
// before the actual output. It handles both single-line and multi-line preamble.
func stripPreamble(text string) string {
	// Known preamble patterns (case-insensitive)
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

// stripAfterDelimiter removes trailing AI commentary by truncating at common
// delimiter patterns that separate the output from meta-commentary.
func stripAfterDelimiter(text string) string {
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Common delimiter patterns: ---  ___  ***
		if trimmed == "---" || trimmed == "___" || trimmed == "***" {
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		}
	}

	return text
}
