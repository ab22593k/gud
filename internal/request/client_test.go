package request

import (
	"context"
	"errors"
	"testing"
)

// mockModelGenerator mocks the GenAI model generation
type mockModelGenerator struct {
	generateContentFunc func(ctx context.Context, model string, parts any, config any) (string, error)
}

func (m *mockModelGenerator) GenerateContent(ctx context.Context, model string, parts any, config any) (string, error) {
	if m.generateContentFunc != nil {
		return m.generateContentFunc(ctx, model, parts, config)
	}
	return "", errors.New("not implemented")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client with mock
			client := &Client{
				model: "gemini-2.5-flash",
			}

			// Note: We can't easily mock the genai.Client without refactoring.
			// For now, test error cases and skip success cases that need API.
			if tt.wantErr && tt.diff == "" {
				msg, err := client.GenerateCommitMessage(context.Background(), tt.diff, tt.context, tt.detailLevel, tt.hint)
				if err == nil {
					t.Errorf("expected error for empty diff")
				}
				if msg != "" {
					t.Errorf("expected empty message for error case")
				}
			} else if !tt.wantErr {
				// Skip API calls in unit tests - would need integration test with mock server
				t.Skip("skipping API call test - needs integration test with mock server")
			}
		})
	}
}
