package request

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/genai"
)

// ContentResponse represents a response from content generation.
type ContentResponse interface {
	Text() string
}

// ModelGenerator defines the interface for AI models that can generate content.
type ModelGenerator interface {
	GenerateContent(ctx context.Context, model string, parts []*genai.Content, config *genai.GenerateContentConfig) (ContentResponse, error)
}

// genaiAdapter adapts genai.Client to the ModelGenerator interface.
type genaiAdapter struct {
	client *genai.Client
}

func (a *genaiAdapter) GenerateContent(ctx context.Context, model string, parts []*genai.Content, config *genai.GenerateContentConfig) (ContentResponse, error) {
	return a.client.Models.GenerateContent(ctx, model, parts, config)
}

// Client wraps the GenAI client for generating commit messages.
type Client struct {
	generator   ModelGenerator
	model       string
	temperature *float32
}

const defaultModel = "gemini-3.1-flash-lite"

// NewClient creates a new request client using the provided API key.
// It uses the Gemini API backend with the specified model, or the default flash model.
// If model is empty, it defaults to "gemini-3-flash-preview".
// If temperature is 0, it is left unset (the API uses the model default).
func NewClient(apiKey, model string, temperature float64) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if model == "" {
		model = defaultModel
	}

	ctx := context.Background()
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	var temp *float32
	if temperature != 0 {
		t := float32(temperature)
		temp = &t
	}

	slog.Debug("created genai client", "model", model, "temperature", temperature)

	return &Client{
		generator:   &genaiAdapter{client: genaiClient},
		model:       model,
		temperature: temp,
	}, nil
}

// NewClientWithGenerator creates a new client with a custom generator for testing.
func NewClientWithGenerator(generator ModelGenerator, model string, temperature float64) *Client {
	if model == "" {
		model = defaultModel
	}
	var temp *float32
	if temperature != 0 {
		t := float32(temperature)
		temp = &t
	}
	return &Client{
		generator:   generator,
		model:       model,
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

	msg, err := generateContent(c, ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if msg == "" {
		return "", fmt.Errorf("generated message is empty")
	}

	return msg, nil
}

func generateContent(c *Client, ctx context.Context, prompt string) (string, error) {
	var config *genai.GenerateContentConfig
	if c.temperature != nil {
		config = &genai.GenerateContentConfig{
			Temperature: c.temperature,
		}
	}
	result, err := c.generator.GenerateContent(
		ctx,
		c.model,
		genai.Text(prompt),
		config,
	)
	if err != nil {
		return "", err
	}
	return result.Text(), nil
}
