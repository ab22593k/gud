package request

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

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

func TestWithDefaultTimeout_PreservesCallerDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	derived, derivedCancel := withDefaultTimeout(ctx, time.Second)
	defer derivedCancel()

	deadline, ok := derived.Deadline()
	if !ok {
		t.Fatal("withDefaultTimeout dropped the caller's deadline")
	}
	// The derived context must carry the caller's deadline (1h), not the 1s default.
	if time.Until(deadline) < 30*time.Minute {
		t.Errorf("derived deadline = %v, want the caller's 1h deadline preserved", deadline)
	}
}

func TestWithDefaultTimeout_AppliesDefaultWhenNoDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := withDefaultTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("withDefaultTimeout did not apply a deadline")
	}
	if time.Until(deadline) > time.Second {
		t.Errorf("deadline = %v, want ~50ms default applied", deadline)
	}
}

func TestGenerateCommitMessageWithContent_RespectsCallerDeadline(t *testing.T) {
	t.Parallel()
	blocking := make(chan struct{})
	defer close(blocking)

	c := NewClientWithGenerator(&mockLLM{
		generateContentFunc: func(ctx context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
			return func(yield func(*model.LLMResponse, error) bool) {
				<-ctx.Done()
				yield(nil, ctx.Err())
			}
		},
	}, "mock")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.GenerateCommitMessageWithContent(ctx, "diff", "", DetailLevel("standard"), "", "", "", 72)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
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
			name:    "valid API key with model",
			cfg:     ClientConfig{APIKey: "test-api-key", Model: "gemini-flash-latest"},
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
	client := NewClientWithGenerator(mock, "test-model")

	if client == nil {
		t.Fatal("NewClientWithGenerator() returned nil")
	}
	if client.model != "test-model" {
		t.Errorf("model = %q, want %q", client.model, "test-model")
	}
	if client.modelImpl != mock {
		t.Errorf("modelImpl should be the mock")
	}
}

func TestSanitizeOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "already clean output passes through",
			in:   "feat: add hello world",
			want: "feat: add hello world",
		},
		{
			name: "multiline commit message preserved",
			in:   "feat: add user auth\n\nImplement login with JWT tokens.\n\nBREAKING CHANGE: auth flow redesigned",
			want: "feat: add user auth\n\nImplement login with JWT tokens.\n\nBREAKING CHANGE: auth flow redesigned",
		},
		{
			name: "strip triple backtick code fences",
			in:   "```\nfeat: add hello world\n```",
			want: "feat: add hello world",
		},
		{
			name: "strip code fences with language tag",
			in:   "```commit\nfeat: add hello world\n```",
			want: "feat: add hello world",
		},
		{
			name: "strip single backtick wrapping",
			in:   "`feat: add hello world`",
			want: "feat: add hello world",
		},
		{
			name: "strip preamble line before commit message",
			in:   "Here is your commit message:\n\nfeat: add hello world",
			want: "feat: add hello world",
		},
		{
			name: "strip 'Here's the commit message:' preamble",
			in:   "Here's the commit message:\nfix: resolve null pointer",
			want: "fix: resolve null pointer",
		},
		{
			name: "strip 'Output:' preamble",
			in:   "Output:\nrefactor: extract validation logic",
			want: "refactor: extract validation logic",
		},
		{
			name: "strip preamble with colon in middle",
			in:   "commit message:\nchore: update deps",
			want: "chore: update deps",
		},
		{
			name: "strip trailing --- delimiter and commentary",
			in:   "feat: add hello world\n\n---\nGenerated by AI",
			want: "feat: add hello world",
		},
		{
			name: "strip trailing ___ delimiter with commentary",
			in:   "feat: add hello world\n\n___\nGenerated by AI",
			want: "feat: add hello world",
		},
		{
			name: "strip trailing *** delimiter with commentary",
			in:   "feat: add hello world\n\n***\nGenerated by AI",
			want: "feat: add hello world",
		},
		{
			name: "strip trailing delimiter with two short commentary lines",
			in:   "feat: add hello world\n\n---\nGenerated by AI\nReview before committing",
			want: "feat: add hello world",
		},
		{
			name: "strip trailing delimiter with blank line before commentary",
			in:   "feat: add hello world\n\n---\n\nGenerated by AI",
			want: "feat: add hello world",
		},
		{
			name: "strip bare trailing separator",
			in:   "feat: add hello world\n\n---",
			want: "feat: add hello world",
		},
		{
			name: "preserve --- with substantive body after it",
			in: "feat: add login\n\nImplement JWT auth with refresh tokens.\n\n---\n\n" +
				"Migration notes:\n- Add users table\n- Backfill existing rows\n- Update auth docs",
			want: "feat: add login\n\nImplement JWT auth with refresh tokens.\n\n---\n\n" +
				"Migration notes:\n- Add users table\n- Backfill existing rows\n- Update auth docs",
		},
		{
			name: "preserve --- with long trailing line",
			in: "feat: add login\n\n---\n" +
				"This line is far longer than any AI commentary would ever be, spanning well beyond one hundred " +
				"characters to ensure it is never mistaken for a trailing note",
			want: "feat: add login\n\n---\n" +
				"This line is far longer than any AI commentary would ever be, spanning well beyond one hundred " +
				"characters to ensure it is never mistaken for a trailing note",
		},
		{
			name: "preserve --- mid-body with blank-line separated paragraph",
			in: "feat: add login\n\nBody paragraph one.\n\n---\n\n" +
				"Body paragraph two is substantive and spans\nacross multiple lines, so it must survive.",
			want: "feat: add login\n\nBody paragraph one.\n\n---\n\n" +
				"Body paragraph two is substantive and spans\nacross multiple lines, so it must survive.",
		},
		{
			name: "preserve --- mid-body with tight but multi-line paragraph",
			in: "feat: add login\n\nBody paragraph one.\n\n---\n" +
				"Body paragraph two is substantive and spans\nacross multiple lines, so it must survive.",
			want: "feat: add login\n\nBody paragraph one.\n\n---\n" +
				"Body paragraph two is substantive and spans\nacross multiple lines, so it must survive.",
		},
		{
			name: "strip only the trailing delimiter when multiple present",
			in:   "feat: add login\n\n---\nGenerated by AI\n\n---\nOne last note",
			want: "feat: add login\n\n---\nGenerated by AI",
		},
		{
			name: "strip leading and trailing whitespace",
			in:   "  \n  feat: add hello world  \n  ",
			want: "feat: add hello world",
		},
		{
			name: "code fences + preamble + whitespace combined",
			in:   "  \n```\nHere is your commit message:\n\nfeat: add hello world\n```\n  ",
			want: "feat: add hello world",
		},
		{
			name: "empty text returns empty",
			in:   "",
			want: "",
		},
		{
			name: "whitespace only returns empty",
			in:   "   \n  \n  ",
			want: "",
		},
		{
			name: "no fence content preserved as-is",
			in:   "fix: resolve memory leak\n\nFreed allocated buffers on error path.",
			want: "fix: resolve memory leak\n\nFreed allocated buffers on error path.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeOutput(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Oracle tests: model name provenance (Comparable + Claims)
// ---------------------------------------------------------------------------

// captureRequestLLM is a mock that captures the LLMRequest for inspection.
type captureRequestLLM struct {
	name     string
	captured *model.LLMRequest
}

func (m *captureRequestLLM) Name() string { return m.name }

func (m *captureRequestLLM) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	m.captured = req
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: genai.NewContentFromText("fix: address issue", "model")}, nil)
	}
}

// TestOracle_Comparable_ModelInRequestMatchesClientModel verifies that the
// model name configured on the Client is faithfully forwarded as req.Model
// to the underlying LLM. A mismatch would mean the API call uses a different
// model than what appears in the Assisted-by trailer.
func TestOracle_Comparable_ModelInRequestMatchesClientModel(t *testing.T) {
	t.Parallel()

	capture := &captureRequestLLM{name: "fake-llm"}
	client := NewClientWithGenerator(capture, "gemini-flash-latest")

	_, err := client.GenerateCommitMessage(context.Background(),
		"diff --git a/main.go b/main.go", "",
		DetailStandard, "", "")
	if err != nil {
		t.Fatalf("GenerateCommitMessage: %v", err)
	}

	if capture.captured == nil {
		t.Fatal("LLM was never called")
	}
	if capture.captured.Model != "gemini-flash-latest" {
		t.Errorf("req.Model = %q, want %q (must match Client.model for Consistent trailer)", capture.captured.Model, "gemini-flash-latest")
	}
}

// TestOracle_Comparable_ModelNameMatchesConfigured verifies that ModelName()
// returns the same value used for the API call, closing the provenance loop:
// config → Client.model → req.Model → ModelName() → Assisted-by trailer.
func TestOracle_Comparable_ModelNameMatchesConfigured(t *testing.T) {
	t.Parallel()

	capture := &captureRequestLLM{name: "fake-llm"}
	client := NewClientWithGenerator(capture, "gemini-flash-latest")

	_, err := client.GenerateCommitMessage(context.Background(),
		"diff --git a/main.go b/main.go", "",
		DetailStandard, "", "")
	if err != nil {
		t.Fatalf("GenerateCommitMessage: %v", err)
	}

	// The model name used in the API call must equal what ModelName() returns.
	if client.ModelName() != capture.captured.Model {
		t.Errorf("ModelName() = %q, req.Model = %q — they MUST agree (Assisted-by trailer would be wrong)", client.ModelName(), capture.captured.Model)
	}
}

// TestOracle_Claims_DefaultModelIsUsedWhenEmpty verifies that an empty model
// string in config defaults to the project constant, and that the default is
// consistently applied to both the API call and the trailer.
func TestOracle_Claims_DefaultModelIsUsedWhenEmpty(t *testing.T) {
	t.Parallel()

	capture := &captureRequestLLM{name: "fake-llm"}
	client := NewClientWithGenerator(capture, "")

	_, err := client.GenerateCommitMessage(context.Background(),
		"diff --git a/main.go b/main.go", "",
		DetailStandard, "", "")
	if err != nil {
		t.Fatalf("GenerateCommitMessage: %v", err)
	}

	if client.ModelName() != defaultModel {
		t.Errorf("ModelName() = %q, want %q (default)", client.ModelName(), defaultModel)
	}
	if capture.captured.Model != defaultModel {
		t.Errorf("req.Model = %q, want %q (default)", capture.captured.Model, defaultModel)
	}
}

func TestClient_GenerateCommitMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		diff          string
		context       string
		detailLevel   DetailLevel
		hint          string
		profile       ProfileName
		mockContent   string
		mockError     string
		mockRespError string
		wantErr       bool
		validateMsg   func(t *testing.T, msg string)
	}{
		{
			name:        "empty diff returns error",
			diff:        "",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
			wantErr:     true,
		},
		{
			name:        "successful generation returns message",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
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
			profile:     "",
			mockContent: "feat: add hello",
			wantErr:     false,
		},
		{
			name:        "with hint provided",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "focus on security",
			profile:     "",
			mockContent: "fix: patch security vulnerability",
			wantErr:     false,
		},
		{
			name:        "API error returns error",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
			mockError:   "API error",
			wantErr:     true,
		},
		{
			name:          "response error message returns error",
			diff:          "diff --git a/main.go b/main.go",
			context:       "",
			detailLevel:   DetailStandard,
			hint:          "",
			profile:       "",
			mockRespError: "API quota exceeded",
			wantErr:       true,
		},
		{
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
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
							yield(nil, errors.New(tt.mockError))
							return
						}
						if tt.mockRespError != "" {
							yield(&model.LLMResponse{
								ErrorMessage: tt.mockRespError,
							}, nil)
							return
						}
						yield(&model.LLMResponse{
							Content: genai.NewContentFromText(tt.mockContent, "model"),
						}, nil)
					}
				},
			}

			client := NewClientWithGenerator(mock, "gemini-flash-lite-latest")

			msg, err := client.GenerateCommitMessage(context.Background(), tt.diff, tt.context, tt.detailLevel, tt.hint, tt.profile)

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
