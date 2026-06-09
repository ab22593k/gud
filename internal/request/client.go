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

// GenerateCommitMessage generates a commit message based on the provided diff, context, detail level, hint, and persona.
func (c *Client) GenerateCommitMessage(ctx context.Context, diff, context string, detailLevel DetailLevel, hint string, persona PersonaName) (string, error) {
	if diff == "" {
		return "", fmt.Errorf("diff is required")
	}

	slog.Debug("generating commit message", "model", c.model, "detailLevel", detailLevel)

	prompt := BuildCommitMessagePrompt(diff, context, detailLevel, hint, persona)

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
	for resp := range c.modelImpl.GenerateContent(ctx, req, false) {
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

// extractText pulls text from genai.Content parts.
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
	return sb.String(), nil
}
