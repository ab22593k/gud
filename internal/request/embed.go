package request

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	// defaultEmbeddingModel is the Gemini embedding model used to vectorise
	// commit diffs and queries. Vectors are pinned to embeddingDimensions via
	// OutputDimensionality, matching the HelixDB Commit.embedding vector
	// index. The model (and therefore the dimension) must never change for an
	// existing index.
	defaultEmbeddingModel = "gemini-embedding-2"

	// embeddingDimensions pins the output vector size. gemini-embedding-2
	// defaults to 3072-dim; it is truncated to 768 to keep the HelixDB vector
	// index (built for text-embedding-004's 768-dim vectors) valid. Google
	// recommends 768, 1536, or 3072 and auto-normalises truncated dims.
	embeddingDimensions = 768

	// embedTextLimit caps input length before embedding so diffs stay within
	// model token limits (gemini-embedding-2 supports 8192 input tokens,
	// roughly 6000 characters of code).
	embedTextLimit = 6000
)

// withDefaultTimeout returns a context with the given timeout if the caller's
// context has no deadline. It preserves explicit caller deadlines so a
// hook-mode or user-visible cancellation is never overridden by the default.
func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

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

	ctx, cancel := withDefaultTimeout(ctx, defaultEmbedTimeout)
	defer cancel()

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
