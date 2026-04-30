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
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "empty API key returns error",
			apiKey:  "",
			wantErr: true,
		},
		{
			name:    "valid API key creates client",
			apiKey:  "test-api-key",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.apiKey)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Errorf("NewClient() should return non-nil client")
			}
		})
	}
}

func TestNewClientWithGenerator(t *testing.T) {
	mock := &mockModelGenerator{}
	client := NewClientWithGenerator(mock, "test-model")

	if client == nil {
		t.Fatal("NewClientWithGenerator() returned nil")
	}
	if client.model != "test-model" {
		t.Errorf("model = %q, want %q", client.model, "test-model")
	}
	if client.generator != mock {
		t.Errorf("generator should be the mock")
	}
}

func TestClient_GenerateCommitMessage(t *testing.T) {
	tests := []struct {
		name         string
		diff         string
		context      string
		detailLevel  string
		hint         string
		mockResponse string
		mockError    error
		wantErr      bool
		validateMsg  func(t *testing.T, msg string)
	}{
		{
			name:        "empty diff returns error",
			diff:        "",
			context:     "",
			detailLevel: "standard",
			hint:        "",
			wantErr:     true,
		},
		{
			name:         "successful generation returns message",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  "standard",
			hint:         "",
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
			detailLevel:  "minimal",
			hint:         "",
			mockResponse: "feat: add hello",
			wantErr:      false,
		},
		{
			name:         "with hint provided",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  "standard",
			hint:         "focus on security",
			mockResponse: "fix: patch security vulnerability",
			wantErr:      false,
		},
		{
			name:        "API error returns error",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: "standard",
			hint:        "",
			mockError:   errors.New("API error"),
			wantErr:     true,
		},
		{
			name:         "empty response returns error",
			diff:         "diff --git a/main.go b/main.go",
			context:      "",
			detailLevel:  "standard",
			hint:         "",
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

			client := NewClientWithGenerator(mock, "gemini-3-flash-preview")

			msg, err := client.GenerateCommitMessage(context.Background(), tt.diff, tt.context, tt.detailLevel, tt.hint, "embedded")

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
