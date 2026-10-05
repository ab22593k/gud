package request

import (
	"context"
	"errors"
	"iter"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
	imodels "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
)

// fakeInteractionCreator stands in for the Interactions service so no test
// reaches the network.
type fakeInteractionCreator struct {
	resp  *operations.CreateInteractionResponse
	err   error
	calls int
	got   operations.CreateInteractionRequest
}

func (f *fakeInteractionCreator) Create(
	_ context.Context, req operations.CreateInteractionRequest, _ ...operations.Option,
) (*operations.CreateInteractionResponse, error) {
	f.calls++
	f.got = req

	return f.resp, f.err
}

func interactionResponse(text string) *operations.CreateInteractionResponse {
	return &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{OutputText: genai.Ptr(text)},
	}
}

// collect drains a model.LLM response iterator, returning the last response and
// the last error. gud consumes a single non-streamed response, so a second
// yield would be a contract violation the caller must not silently accept.
func collect(t *testing.T, seq iter.Seq2[*model.LLMResponse, error]) (*model.LLMResponse, error, int) {
	t.Helper()

	var (
		resp  *model.LLMResponse
		err   error
		count int
	)

	for r, e := range seq {
		resp, err = r, e
		count++
	}

	return resp, err, count
}

func llmRequest(prompt, modelName string) *model.LLMRequest {
	return &model.LLMRequest{
		Model:    modelName,
		Contents: genai.Text(prompt),
		Config:   &genai.GenerateContentConfig{},
	}
}

func TestInteractionsModel_ReturnsOutputText(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("feat: add login endpoint")}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	resp, err, count := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 1 {
		t.Errorf("yielded %d responses, want exactly 1", count)
	}

	if got := resp.Content.Parts[0].Text; got != "feat: add login endpoint" {
		t.Errorf("text = %q, want %q", got, "feat: add login endpoint")
	}
}

// TestInteractionsModel_ReadsTextFromModelOutputSteps is the regression lock for
// the real response shape.
//
// A live call returns the commit message inside the interaction's steps array, as
// a step of type "model_output" whose content carries TextContent. The
// convenience OutputText field is left nil for this shape, so reading only that
// field yields an empty message — which is exactly the "generated message is
// empty" failure this test was written to prevent.
func TestInteractionsModel_ReadsTextFromModelOutputSteps(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{
			Steps: []imodels.Step{
				// A thought step carries a signature, not text, and must be ignored.
				{Type: imodels.StepTypeThought},
				{
					Type: imodels.StepTypeModelOutput,
					ModelOutputStep: &imodels.ModelOutputStep{
						Content: []imodels.Content{
							{TextContent: &imodels.TextContent{Text: "feat: add login endpoint"}},
						},
					},
				},
			},
		},
	}}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := resp.Content.Parts[0].Text; got != "feat: add login endpoint" {
		t.Errorf("text = %q, want %q", got, "feat: add login endpoint")
	}
}

// A diff large enough to produce several content parts arrives as several
// contents within the model_output step; all of them are the message.
func TestInteractionsModel_ConcatenatesModelOutputContents(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{
			Steps: []imodels.Step{
				{
					Type: imodels.StepTypeModelOutput,
					ModelOutputStep: &imodels.ModelOutputStep{
						Content: []imodels.Content{
							{TextContent: &imodels.TextContent{Text: "feat: add login endpoint\n"}},
							{TextContent: &imodels.TextContent{Text: "Handles expired tokens."}},
						},
					},
				},
			},
		},
	}}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "feat: add login endpoint\nHandles expired tokens."
	if got := resp.Content.Parts[0].Text; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// OutputText is still a real field on some response shapes, so it stays
// supported as a fallback rather than being deleted.
func TestInteractionsModel_FallsBackToOutputText(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("chore: bump deps")}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := resp.Content.Parts[0].Text; got != "chore: bump deps" {
		t.Errorf("text = %q, want %q", got, "chore: bump deps")
	}
}

// A completed interaction carrying no model_output step at all is a protocol
// violation worth surfacing, not a silently empty commit message.
func TestInteractionsModel_NoModelOutputStepIsAnError(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{
			Steps: []imodels.Step{{Type: imodels.StepTypeThought}},
		},
	}}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	_, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err == nil {
		t.Fatal("expected an error when no model_output step is present, got nil")
	}
}

func TestInteractionsModel_PropagatesCreatorError(t *testing.T) {
	t.Parallel()

	boom := errors.New("API error occurred: Status 429\nquota exceeded")
	fake := &fakeInteractionCreator{err: boom}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	_, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the creator error to survive unwrapping", err)
	}
}

// A successful HTTP call carrying no interaction must not be read as an empty
// commit message; it is an upstream protocol violation.
func TestInteractionsModel_NilInteractionIsError(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{}}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	_, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err == nil {
		t.Fatal("expected an error for a response with no interaction, got nil")
	}
}

// A model_output step that carries no text is the caller's business to reject
// with its own "message is empty" error. The adapter must not second-guess it —
// only a missing model_output step is a protocol violation (covered separately).
func TestInteractionsModel_EmptyOutputIsNotAnError(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{
			Steps: []imodels.Step{{
				Type:            imodels.StepTypeModelOutput,
				ModelOutputStep: &imodels.ModelOutputStep{Content: []imodels.Content{}},
			}},
		},
	}}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content == nil {
		t.Fatal("expected non-nil content so the caller reports 'message is empty'")
	}

	if got := resp.Content.Parts[0].Text; got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

func TestInteractionsModel_PrefersRequestModelThenFallsBack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		reqModel   string
		configured string
		want       string
	}{
		{"request model wins", "gemini-2.5-pro", "gemini-flash-lite-latest", "gemini-2.5-pro"},
		{"falls back to configured", "", "gemini-flash-lite-latest", "gemini-flash-lite-latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeInteractionCreator{resp: interactionResponse("fix: repair cache")}
			m := newInteractionsModel(tc.configured, fake)

			collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", tc.reqModel), false))

			got := fake.got.Body.CreateModelInteraction.Model
			if string(got) != tc.want {
				t.Errorf("model = %q, want %q", got, tc.want)
			}
		})
	}
}

// The prompt reaches the service as a single string; multi-part content must be
// concatenated rather than truncated to the first part.
func TestInteractionsModel_ConcatenatesPromptParts(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("chore: bump deps")}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	req := &model.LLMRequest{
		Model: "gemini-flash-lite-latest",
		Contents: []*genai.Content{
			{Parts: []*genai.Part{{Text: "first "}}},
			{Parts: []*genai.Part{{Text: "second"}}},
		},
		Config: &genai.GenerateContentConfig{},
	}

	collect(t, m.GenerateContent(context.Background(), req, false))

	input := fake.got.Body.CreateModelInteraction.Input
	if input == nil || input.Str == nil {
		t.Fatal("expected the prompt to be sent as a string input")
	}

	if *input.Str != "first second" {
		t.Errorf("input = %q, want %q", *input.Str, "first second")
	}
}

func TestInteractionsModel_Name(t *testing.T) {
	t.Parallel()

	m := newInteractionsModel("gemini-2.5-pro", &fakeInteractionCreator{})
	if got := m.Name(); got != "gemini-2.5-pro" {
		t.Errorf("Name() = %q, want %q", got, "gemini-2.5-pro")
	}
}

// Compile-time proof that the adapter is drop-in for the agent toolkit seam,
// which is what lets every existing call site and mock stay unchanged.
func TestInteractionsModel_SatisfiesLLM(t *testing.T) {
	t.Parallel()

	var _ model.LLM = (*interactionsModel)(nil)
}

// The creator must receive exactly one call per generation, so a retry loop
// cannot silently multiply requests.
func TestInteractionsModel_CreatesOncePerGeneration(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("docs: clarify usage")}
	m := newInteractionsModel("gemini-flash-lite-latest", fake)

	collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), true))

	if fake.calls != 1 {
		t.Errorf("creator called %d times, want exactly 1", fake.calls)
	}
}
