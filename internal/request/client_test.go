package request

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

// mockModelGenerator implements ModelGenerator for testing.
type mockModelGenerator struct {
	generateContentFunc func(ctx context.Context, model string, parts []*genai.Content, config *genai.GenerateContentConfig) (ContentResponse, error)
}

func (m *mockModelGenerator) GenerateContent(ctx context.Context, model string, parts []*genai.Content, config *genai.GenerateContentConfig) (ContentResponse, error) {
	if m.generateContentFunc != nil {
		return m.generateContentFunc(ctx, model, parts, config)
	}
	return nil, errors.New("not implemented")
}

// mockContentResponse implements ContentResponse for testing.
type mockContentResponse struct {
	text string
}

func (m *mockContentResponse) Text() string {
	return m.text
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		model       string
		temperature float64
		wantErr     bool
	}{
		{
			name:    "empty API key returns error",
			apiKey:  "",
			model:   "",
			wantErr: true,
		},
		{
			name:    "valid API key with no model uses default",
			apiKey:  "test-api-key",
			model:   "",
			wantErr: false,
		},
		{
			name:    "valid API key with custom model",
			apiKey:  "test-api-key",
			model:   "gemini-2.5-pro",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.apiKey, tt.model, tt.temperature)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Errorf("NewClient() should return non-nil client")
			}
			if !tt.wantErr && tt.model != "" && client.model != tt.model {
				t.Errorf("client.model = %q, want %q", client.model, tt.model)
			}
			if !tt.wantErr && tt.model == "" && client.model != defaultModel {
				t.Errorf("client.model = %q, want default %q", client.model, defaultModel)
			}
		})
	}
}

func TestNewClientWithGenerator(t *testing.T) {
	mock := &mockModelGenerator{}
	client := NewClientWithGenerator(mock, "test-model", 0)

	if client == nil {
		t.Fatal("NewClientWithGenerator() returned nil")
	}
	if client.model != "test-model" {
		t.Errorf("model = %q, want %q", client.model, "test-model")
	}
	if client.generator != mock {
		t.Errorf("generator should be the mock")
	}
	if client.temperature != nil {
		t.Errorf("temperature should be nil for default 0")
	}
}

func TestClient_GenerateCommitMessage(t *testing.T) {
	tests := []struct {
		name         string
		diff         string
		context      string
		detailLevel  DetailLevel
		hint         string
		persona      PersonaName
		mockResponse string
		mockError    error
		wantErr      bool
		validateMsg  func(t *testing.T, msg string)
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
			name:         "successful generation returns message",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  DetailStandard,
			hint:         "",
			persona:      PersonaEmbedded,
			mockResponse: "feat: add hello world output",
			wantErr:      false,
			validateMsg: func(t *testing.T, msg string) {
				if msg != "feat: add hello world output" {
					t.Errorf("got %q, want %q", msg, "feat: add hello world output")
				}
			},
		},
		{
			name:         "with minimal detail level",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  DetailMinimal,
			hint:         "",
			persona:      PersonaEmbedded,
			mockResponse: "feat: add hello",
			wantErr:      false,
		},
		{
			name:         "with hint provided",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  DetailStandard,
			hint:         "focus on security",
			persona:      PersonaEmbedded,
			mockResponse: "fix: patch security vulnerability",
			wantErr:      false,
		},
		{
			name:        "API error returns error",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
			mockError:   errors.New("API error"),
			wantErr:     true,
		},
		{
			name:         "empty response returns error",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  DetailStandard,
			hint:         "",
			persona:      PersonaEmbedded,
			mockResponse: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockModelGenerator{
				generateContentFunc: func(ctx context.Context, model string, parts []*genai.Content, config *genai.GenerateContentConfig) (ContentResponse, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return &mockContentResponse{text: tt.mockResponse}, nil
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
