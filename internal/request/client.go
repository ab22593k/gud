package request

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// Client wraps the GenAI client for generating commit messages.
type Client struct {
	genaiClient *genai.Client
	model       string
}

// NewClient creates a new request client using the provided API key.
// It uses the Gemini API backend with the flash model for fast responses.
func NewClient(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &Client{
		genaiClient: client,
		model:       "gemini-2.5-flash",
	}, nil
}

// GenerateCommitMessage generates a commit message based on the provided diff, context, detail level, and hint.
func (c *Client) GenerateCommitMessage(ctx context.Context, diff, context, detailLevel, hint string) (string, error) {
	if diff == "" {
		return "", fmt.Errorf("diff is required")
	}

	prompt := BuildCommitMessagePrompt(diff, context, detailLevel, hint)

	result, err := c.genaiClient.Models.GenerateContent(
		ctx,
		c.model,
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	msg := result.Text()
	if msg == "" {
		return "", fmt.Errorf("generated message is empty")
	}

	return msg, nil
}
