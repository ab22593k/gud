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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "feat: add login endpoint\nHandles expired tokens."
	if got := resp.Content.Parts[0].Text; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// An Antigravity run walks a tool-use loop that emits a model_output step per
// reasoning turn. Only the last step is the answer: splicing every step
// together would prepend the agent's intermediate turns to the commit message.
func TestInteractionsModel_ReadsTextFromLastModelOutputStep(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{
			Steps: []imodels.Step{
				{Type: imodels.StepTypeThought},
				{
					Type: imodels.StepTypeModelOutput,
					ModelOutputStep: &imodels.ModelOutputStep{
						Content: []imodels.Content{
							{TextContent: &imodels.TextContent{Text: "I will inspect the diff first."}},
						},
					},
				},
				{Type: imodels.StepTypeCodeExecutionCall},
				{Type: imodels.StepTypeCodeExecutionResult},
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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "feat: add login endpoint\nHandles expired tokens."
	if got := resp.Content.Parts[0].Text; got != want {
		t.Errorf("text = %q, want %q (intermediate turns must be dropped)", got, want)
	}
}

// OutputText is the SDK's own aggregation of the last model output, so it wins
// over the steps array when the service populates it — which is what the
// Antigravity agent does.
func TestInteractionsModel_PrefersOutputTextOverSteps(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: &operations.CreateInteractionResponse{
		Interaction: &imodels.Interaction{
			OutputText: genai.Ptr("feat: add login endpoint"),
			Steps: []imodels.Step{
				{
					Type: imodels.StepTypeModelOutput,
					ModelOutputStep: &imodels.ModelOutputStep{
						Content: []imodels.Content{
							{TextContent: &imodels.TextContent{Text: "an earlier turn"}},
						},
					},
				},
			},
		},
	}}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	resp, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := resp.Content.Parts[0].Text; got != "feat: add login endpoint" {
		t.Errorf("text = %q, want %q", got, "feat: add login endpoint")
	}
}

// OutputText is still a real field on some response shapes, so it stays
// supported as a fallback rather than being deleted.
func TestInteractionsModel_FallsBackToOutputText(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("chore: bump deps")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	_, err, _ := collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))
	if err == nil {
		t.Fatal("expected an error when no model_output step is present, got nil")
	}
}

func TestInteractionsModel_PropagatesCreatorError(t *testing.T) {
	t.Parallel()

	boom := errors.New("API error occurred: Status 429\nquota exceeded")
	fake := &fakeInteractionCreator{err: boom}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

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
		{"request model wins", "gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-3.5-flash"},
		{"falls back to configured", "", "gemini-3.5-flash-lite", "gemini-3.5-flash-lite"},
		// The agent serves a fixed model set; a name outside it resolves to the
		// agent's own default rather than failing every request.
		{"unavailable model falls back to agent default", "gemini-2.5-pro", "gemini-3.5-flash-lite", "gemini-3.8-flash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeInteractionCreator{resp: interactionResponse("fix: repair cache")}
			m := newInteractionsModel(tc.configured, fake)

			collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", tc.reqModel), false))

			got := fake.got.Body.CreateAgentInteraction.GetAgentConfigAntigravity().GetModel()
			if got == nil {
				t.Fatal("agent_config.model was not set")
			}

			if *got != tc.want {
				t.Errorf("agent_config.model = %q, want %q", *got, tc.want)
			}
		})
	}
}

// Every generation runs on the Antigravity agent inside a remote environment;
// either one missing turns the request into a plain model call (or a rejected
// one), which this adapter is no longer written for.
func TestInteractionsModel_UsesAntigravityAgent(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("feat: add login endpoint")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))

	agent := fake.got.Body.CreateAgentInteraction
	if agent.Agent != antigravityAgent {
		t.Errorf("agent = %q, want %q", agent.Agent, antigravityAgent)
	}

	env := agent.GetEnvironment()
	if env == nil || env.Str == nil {
		t.Fatal("environment was not sent")
	}

	if *env.Str != remoteEnvironment {
		t.Errorf("environment = %q, want %q", *env.Str, remoteEnvironment)
	}
}

// Mounted files become inline sources at the paths they were given, so a
// monorepo's tree of nested AGENTS.md files is reproduced in the sandbox.
func TestInteractionsModel_MountsFilesAtTheirPaths(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("feat: add login endpoint")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	mounts := []AgentFile{
		{Path: RootMountPath, Content: "root conventions"},
		{Path: "packages/ui/" + RootMountPath, Content: "ui conventions"},
	}

	collect(t, m.GenerateWithMounts(context.Background(), llmRequest("prompt", ""), mounts))

	env := fake.got.Body.CreateAgentInteraction.GetEnvironment()
	if env == nil || env.Environment == nil {
		t.Fatal("expected an environment object with sources, got a bare string")
	}

	sources := env.Environment.Sources
	if len(sources) != len(mounts) {
		t.Fatalf("sources = %d, want %d", len(sources), len(mounts))
	}

	for i, want := range mounts {
		got := sources[i]
		if got.Type == nil || *got.Type != imodels.SourceTypeInline {
			t.Errorf("source %d type = %v, want inline", i, got.Type)
		}

		if got.Target == nil || *got.Target != want.Path {
			t.Errorf("source %d target = %v, want %q", i, got.Target, want.Path)
		}

		if got.Content == nil || *got.Content != want.Content {
			t.Errorf("source %d content = %v, want %q", i, got.Content, want.Content)
		}
	}
}

// The nearest file occupies the root slot, which is the one the Antigravity
// runtime loads as system instructions — that is what makes it take precedence.
func TestInteractionsModel_NearestFileTakesRootSlot(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("feat: add login endpoint")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	collect(t, m.GenerateWithMounts(context.Background(), llmRequest("prompt", ""),
		[]AgentFile{{Path: RootMountPath, Content: "nearest conventions"}}))

	env := fake.got.Body.CreateAgentInteraction.GetEnvironment()
	if env == nil || env.Environment == nil || len(env.Environment.Sources) != 1 {
		t.Fatal("expected exactly one mounted source")
	}

	if got := env.Environment.Sources[0].GetTarget(); got == nil || *got != RootMountPath {
		t.Errorf("target = %v, want %q", got, RootMountPath)
	}
}

// Without files the environment stays the bare "remote" string: no sources are
// sent, so a generation with nothing to mount is byte-identical to one that
// never had the feature.
func TestInteractionsModel_NoMountsSendsBareRemoteEnvironment(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("feat: add login endpoint")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), false))

	env := fake.got.Body.CreateAgentInteraction.GetEnvironment()
	if env == nil || env.Str == nil {
		t.Fatalf("environment = %v, want the bare %q string", env, remoteEnvironment)
	}

	if *env.Str != remoteEnvironment {
		t.Errorf("environment = %q, want %q", *env.Str, remoteEnvironment)
	}
}

// An empty mount list is the same as no list at all, not an environment with an
// empty sources array.
func TestInteractionsModel_EmptyMountListSendsBareRemoteEnvironment(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("feat: add login endpoint")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	collect(t, m.GenerateWithMounts(context.Background(), llmRequest("prompt", ""), []AgentFile{}))

	env := fake.got.Body.CreateAgentInteraction.GetEnvironment()
	if env == nil || env.Str == nil || *env.Str != remoteEnvironment {
		t.Errorf("environment = %v, want the bare %q string", env, remoteEnvironment)
	}
}

// The prompt reaches the service as a single string; multi-part content must be
// concatenated rather than truncated to the first part.
func TestInteractionsModel_ConcatenatesPromptParts(t *testing.T) {
	t.Parallel()

	fake := &fakeInteractionCreator{resp: interactionResponse("chore: bump deps")}
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	req := &model.LLMRequest{
		Model: "gemini-3.5-flash-lite",
		Contents: []*genai.Content{
			{Parts: []*genai.Part{{Text: "first "}}},
			{Parts: []*genai.Part{{Text: "second"}}},
		},
		Config: &genai.GenerateContentConfig{},
	}

	collect(t, m.GenerateContent(context.Background(), req, false))

	input := fake.got.Body.CreateAgentInteraction.Input
	if input == nil || input.Str == nil {
		t.Fatal("expected the prompt to be sent as a string input")
	}

	if *input.Str != "first second" {
		t.Errorf("input = %q, want %q", *input.Str, "first second")
	}
}

func TestInteractionsModel_Name(t *testing.T) {
	t.Parallel()

	m := newInteractionsModel("gemini-3.6-flash", &fakeInteractionCreator{})
	if got := m.Name(); got != "gemini-3.6-flash" {
		t.Errorf("Name() = %q, want %q", got, "gemini-3.6-flash")
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
	m := newInteractionsModel("gemini-3.5-flash-lite", fake)

	collect(t, m.GenerateContent(context.Background(), llmRequest("prompt", ""), true))

	if fake.calls != 1 {
		t.Errorf("creator called %d times, want exactly 1", fake.calls)
	}
}
