package request

import (
	"context"
	"iter"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// mockLLM implements model.LLM for testing.
type mockLLM struct {
	generateContentFunc func(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error]
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if m.generateContentFunc != nil {
		return m.generateContentFunc(ctx, req, stream)
	}
	// Return empty response
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{}, nil)
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
	}{
		{
			name:    "empty API key returns error",
			cfg:     ClientConfig{APIKey: ""},
			wantErr: true,
		},
		{
			name:    "valid API key with no model uses default",
			cfg:     ClientConfig{APIKey: "test-api-key"},
			wantErr: false,
		},
		{
			name:    "valid API key with gemini provider",
			cfg:     ClientConfig{APIKey: "test-api-key", Model: "gemini-2.5-pro", ACP: "gemini"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, err := NewClient(context.Background(), tt.cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Errorf("NewClient() should return non-nil client")
			}
			if !tt.wantErr && tt.cfg.Model != "" && client.model != tt.cfg.Model {
				t.Errorf("client.model = %q, want %q", client.model, tt.cfg.Model)
			}
		})
	}
}

func TestNewClientWithGenerator(t *testing.T) {
	t.Parallel()
	mock := &mockLLM{}
	client := NewClientWithGenerator(mock, "test-model", 0)

	if client == nil {
		t.Fatal("NewClientWithGenerator() returned nil")
	}
	if client.model != "test-model" {
		t.Errorf("model = %q, want %q", client.model, "test-model")
	}
	if client.modelImpl != mock {
		t.Errorf("modelImpl should be the mock")
	}
	if client.temperature != nil {
		t.Errorf("temperature should be nil for default 0")
	}
}

func TestClient_GenerateCommitMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		diff        string
		context     string
		detailLevel DetailLevel
		hint        string
		persona     PersonaName
		mockContent string
		mockError   string
		wantErr     bool
		validateMsg func(t *testing.T, msg string)
	}{
		{
			name:        "empty diff returns error",
			diff:        "",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
			wantErr:     true,
		},
		{
			name:        "successful generation returns message",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
			mockContent: "feat: add hello world output",
			wantErr:     false,
			validateMsg: func(t *testing.T, msg string) {
				if msg != "feat: add hello world output" {
					t.Errorf("got %q, want %q", msg, "feat: add hello world output")
				}
			},
		},
		{
			name:        "with minimal detail level",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailMinimal,
			hint:        "",
			persona:     PersonaEmbedded,
			mockContent: "feat: add hello",
			wantErr:     false,
		},
		{
			name:        "with hint provided",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "focus on security",
			persona:     PersonaEmbedded,
			mockContent: "fix: patch security vulnerability",
			wantErr:     false,
		},
		{
			name:        "API error returns error",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
			mockError:   "API error",
			wantErr:     true,
		},
		{
			name:        "empty response returns error",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
			mockContent: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mock := &mockLLM{
				generateContentFunc: func(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
					return func(yield func(*model.LLMResponse, error) bool) {
						if tt.mockError != "" {
							yield(&model.LLMResponse{
								Content:      nil,
								ErrorMessage: tt.mockError,
							}, nil)
							return
						}
						yield(&model.LLMResponse{
							Content: genai.NewContentFromText(tt.mockContent, "model"),
						}, nil)
					}
				},
			}

			client := NewClientWithGenerator(mock, "gemini-3.1-flash-lite", 0)

			msg, err := client.GenerateCommitMessage(context.Background(), tt.diff, tt.context, tt.detailLevel, tt.hint, tt.persona)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateCommitMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validateMsg != nil {
				tt.validateMsg(t, msg)
			}
		})
	}
}
