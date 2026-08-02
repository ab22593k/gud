package request

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClient_EmbedText(t *testing.T) {
	t.Parallel()

	t.Run("no embedder returns error", func(t *testing.T) {
		t.Parallel()
		c := NewClientWithGenerator(&mockLLM{}, "test-model")
		if _, err := c.EmbedText(context.Background(), "some diff"); err == nil {
			t.Error("expected error when no embedder configured")
		}
	})

	t.Run("empty text returns error", func(t *testing.T) {
		t.Parallel()
		c := NewClientWithGenerator(&mockLLM{}, "test-model")
		c.SetEmbedder(func(_ context.Context, _ string) ([]float32, error) {
			return []float32{1, 2}, nil
		})
		if _, err := c.EmbedText(context.Background(), "   \n "); err == nil {
			t.Error("expected error for empty text")
		}
	})

	t.Run("embeds trimmed text", func(t *testing.T) {
		t.Parallel()
		var got string
		c := NewClientWithGenerator(&mockLLM{}, "test-model")
		c.SetEmbedder(func(_ context.Context, text string) ([]float32, error) {
			got = text
			return []float32{0.5, 0.5}, nil
		})

		vec, err := c.EmbedText(context.Background(), "  feat: trim me  ")
		if err != nil {
			t.Fatalf("EmbedText: %v", err)
		}
		if len(vec) != 2 || vec[0] != 0.5 {
			t.Errorf("unexpected vector: %v", vec)
		}
		if got != "feat: trim me" {
			t.Errorf("embedder received %q, want trimmed input", got)
		}
	})

	t.Run("long text is truncated", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("x", embedTextLimit+100)
		c := NewClientWithGenerator(&mockLLM{}, "test-model")
		c.SetEmbedder(func(_ context.Context, text string) ([]float32, error) {
			if len(text) != embedTextLimit {
				t.Errorf("expected truncation to %d chars, got %d", embedTextLimit, len(text))
			}
			return []float32{0.1}, nil
		})
		if _, err := c.EmbedText(context.Background(), long); err != nil {
			t.Fatalf("EmbedText: %v", err)
		}
	})

	t.Run("propagates embedder error", func(t *testing.T) {
		t.Parallel()
		c := NewClientWithGenerator(&mockLLM{}, "test-model")
		c.SetEmbedder(func(_ context.Context, _ string) ([]float32, error) {
			return nil, errors.New("api down")
		})
		if _, err := c.EmbedText(context.Background(), "diff"); err == nil {
			t.Error("expected embedder error to propagate")
		}
	})
}

func TestClient_EmbeddingModelDefault(t *testing.T) {
	t.Parallel()

	t.Run("empty config uses default model", func(t *testing.T) {
		c := NewClientWithGenerator(&mockLLM{}, "test-model")
		if c.EmbeddingModel() != "" {
			t.Errorf("test client EmbeddingModel() = %q, want empty", c.EmbeddingModel())
		}
		if defaultEmbeddingModel == "" {
			t.Fatal("defaultEmbeddingModel must not be empty")
		}
	})
}

func TestEmbedTextLimitConstant(t *testing.T) {
	if embedTextLimit <= 0 {
		t.Fatalf("embedTextLimit = %d, want > 0", embedTextLimit)
	}
}

func TestDefaultEmbeddingModel(t *testing.T) {
	t.Parallel()

	// text-embedding-004 was shut down by Google on 2026-01-14; the default
	// must never point at a retired model, otherwise vector recall silently
	// degrades to BM25-only (as the API returns 404 NOT_FOUND).
	if defaultEmbeddingModel == "text-embedding-004" {
		t.Fatal("defaultEmbeddingModel must not be the retired text-embedding-004")
	}
	if strings.TrimSpace(defaultEmbeddingModel) == "" {
		t.Fatal("defaultEmbeddingModel must not be empty")
	}
}

func TestEmbeddingDimensionsIndexCompatibility(t *testing.T) {
	t.Parallel()

	// The HelixDB Commit.embedding vector index was created for 768-dim
	// vectors (text-embedding-004). Changing the dimension invalidates the
	// index, so embeddingDimensions is pinned to 768 and must never change.
	if embeddingDimensions != 768 {
		t.Fatalf("embeddingDimensions = %d, want 768 to match the HelixDB vector index", embeddingDimensions)
	}
}
