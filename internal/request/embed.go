package request

import (
	"context"
	"fmt"
	"strings"
)

const (
	// defaultEmbeddingModel is the Gemini embedding model used to vectorise
	// commit diffs and queries. It produces fixed 768-dim vectors, matching
	// the HelixDB Commit.embedding vector index. The model (and therefore the
	// dimension) must never change for an existing index.
	defaultEmbeddingModel = "text-embedding-004"

	// embedTextLimit caps input length before embedding so diffs stay within
	// model token limits (text-embedding-004 supports 2048 input tokens,
	// roughly 6000 characters of code).
	embedTextLimit = 6000
)

// EmbedText embeds the given text as a float32 vector using the configured
// Gemini embedding model. Input is trimmed, must be non-empty, and is
// truncated to embedTextLimit characters to respect model token limits.
// Clients without an embedder (e.g. test doubles) return an error.
func (c *Client) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if c.embedFn == nil {
		return nil, fmt.Errorf("embeddings: not available for this client")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embeddings: empty text")
	}
	if len(text) > embedTextLimit {
		text = text[:embedTextLimit]
	}
	return c.embedFn(ctx, text)
}

// SetEmbedder overrides the embedding function. Used by tests to avoid real
// API calls; passing nil clears the override.
func (c *Client) SetEmbedder(fn func(context.Context, string) ([]float32, error)) {
	c.embedFn = fn
}

// EmbeddingModel returns the embedding model name configured on the client.
func (c *Client) EmbeddingModel() string {
	return c.embeddingModel
}
