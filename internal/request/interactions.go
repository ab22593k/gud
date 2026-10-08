package request

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
	imodels "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
)

const (
	// antigravityAgent is the managed agent that runs every generation: it
	// reasons, executes code, and manages files inside a remote sandbox.
	//
	// Spelled out rather than taken from the SDK's AgentOption enum because the
	// pinned genai release predates this agent — its enum carries only
	// antigravity-preview-05-2026. AgentOption is a plain string type, so the
	// documented value marshals and sends unchanged.
	//
	// https://ai.google.dev/gemini-api/docs/antigravity-agent
	antigravityAgent imodels.AgentOption = "antigravity-preview-09-2026"

	// remoteEnvironment provisions the agent's sandbox. Setting it is what
	// enables the filesystem tools and returns the environment_id a follow-up
	// turn reattaches to.
	remoteEnvironment = "remote"

	// RootMountPath is the environment-root location the Antigravity runtime
	// loads as system instructions on startup. Discovery puts the nearest
	// AGENTS.md there, which is what makes the closest file take precedence
	// over every other one mounted.
	RootMountPath = "AGENTS.md"

	// agentDefaultModel is the Antigravity agent's own default model, applied
	// when the configured model is not one the agent offers.
	agentDefaultModel = "gemini-3.8-flash"
)

// antigravityModels lists the models the Antigravity agent accepts in
// agent_config.model. Any other value is rejected by the API, so a configured
// name outside this set is replaced by agentDefaultModel rather than failing
// every request.
//
// https://ai.google.dev/gemini-api/docs/antigravity-agent#model-selection
var antigravityModels = []string{
	"gemini-3.8-flash",
	"gemini-3.7-flash",
	"gemini-3.6-flash",
	"gemini-3.5-flash",
	"gemini-3.5-flash-lite",
}

// AgentFile is one AGENTS.md to mount into the agent's environment. Path is
// the slash-separated location it occupies there — relative to the environment
// root, or the root itself for the nearest file — and Content is its text.
type AgentFile struct {
	Path    string
	Content string
}

// Generator is the seam Client uses to reach the model.
//
// It extends model.LLM with the AGENTS.md mount list because the ADK signature
// carries no field for one: GenerateContent's *model.LLMRequest can hold a
// system instruction, but not a set of files to place in the agent's
// environment. Adapters that mount nothing ignore the list.
type Generator interface {
	model.LLM

	GenerateWithMounts(
		ctx context.Context, req *model.LLMRequest, mounts []AgentFile,
	) iter.Seq2[*model.LLMResponse, error]
}

// plainGenerator adapts a model.LLM into a Generator that discards mounts, for
// callers that only exercise prompt construction.
type plainGenerator struct {
	model.LLM
}

func (p plainGenerator) GenerateWithMounts(
	ctx context.Context, req *model.LLMRequest, _ []AgentFile,
) iter.Seq2[*model.LLMResponse, error] {
	return p.LLM.GenerateContent(ctx, req, false)
}

// interactionCreator is the slice of the Interactions API this adapter uses.
// *interactions.Interactions satisfies it; tests substitute a fake so no
// generation reaches the network.
type interactionCreator interface {
	Create(
		ctx context.Context, req operations.CreateInteractionRequest, opts ...operations.Option,
	) (*operations.CreateInteractionResponse, error)
}

// interactionsModel routes commit-message generation through the Gemini
// Antigravity agent while presenting the agent toolkit's model.LLM interface.
//
// Presenting that interface is deliberate: Client.modelImpl is typed as
// Generator, which model.LLM satisfies, so every call site and every test mock
// stays unchanged. The mount-aware suffix exists only because the ADK signature
// has no field for a file list.
//
// Each generation is a single independent interaction. No previous-interaction
// reference is sent, so no conversation state accumulates across invocations
// and the agent's remote environment is never reused between runs.
type interactionsModel struct {
	name    string
	creator interactionCreator
}

// newInteractionsModel returns a model.LLM backed by the Antigravity agent.
func newInteractionsModel(name string, creator interactionCreator) *interactionsModel {
	return &interactionsModel{name: name, creator: creator}
}

// Name reports the model this adapter was configured with.
func (m *interactionsModel) Name() string { return m.name }

// GenerateContent performs one interaction and yields exactly one aggregated
// response. It is model.LLM's entry point; the mount-aware path is
// GenerateWithMounts, which this delegates to with nothing to mount.
//
// The stream argument is accepted to satisfy model.LLM but does not select
// partial delivery: gud always consumes a single complete message, and a
// half-written commit message — or a partial agent turn that still has tool
// calls to make — would be worse than waiting for the run to finish.
func (m *interactionsModel) GenerateContent(
	ctx context.Context, req *model.LLMRequest, _ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return m.GenerateWithMounts(ctx, req, nil)
}

// GenerateWithMounts performs one interaction, mounting the given AGENTS.md
// files into the agent's environment, and yields exactly one aggregated
// response.
func (m *interactionsModel) GenerateWithMounts(
	ctx context.Context, req *model.LLMRequest, mounts []AgentFile,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.create(ctx, req, mounts)
		if err != nil {
			yield(nil, err)

			return
		}

		yield(resp, nil)
	}
}

// create performs the interaction and converts the result into a model.LLMResponse.
func (m *interactionsModel) create(
	ctx context.Context, req *model.LLMRequest, mounts []AgentFile,
) (*model.LLMResponse, error) {
	res, err := m.creator.Create(ctx, operations.CreateInteractionRequest{
		Body: operations.NewCreateInteractionRequestBody(imodels.CreateAgentInteraction{
			Agent:       antigravityAgent,
			Input:       genai.Ptr(imodels.NewInteractionsInput(promptText(req.Contents))),
			Environment: genai.Ptr(agentEnvironment(mounts)),
			AgentConfig: agentConfig(m.resolveModel(req)),
		}),
	})
	if err != nil {
		// Wrapped rather than replaced: isTransientErr classifies on the
		// message, and the Interactions client renders the HTTP status into it
		// ("API error occurred: Status 429"). Unwrapping keeps that intact.
		return nil, fmt.Errorf("model error: %w", err)
	}

	if res == nil || res.Interaction == nil {
		return nil, errors.New("model returned no interaction")
	}

	text, err := interactionText(res.Interaction)
	if err != nil {
		return nil, err
	}

	return &model.LLMResponse{
		Content: genai.NewContentFromText(text, "model"),
	}, nil
}

// interactionText extracts the model's reply from a completed interaction.
//
// OutputText is the SDK's own aggregation — "concatenated text from the last
// model output" — so it is read first. That matters here: an Antigravity run
// walks a tool-use loop that emits a model_output step per reasoning turn, and
// only the last one is the answer. Reading every step instead would splice the
// agent's intermediate turns into the commit message.
//
// The steps array is the fallback for response shapes that leave OutputText
// unset. There the message is the last step of type "model_output", with its
// content parts concatenated; thought and tool-call steps carry no user-facing
// text and are skipped.
func interactionText(interaction *imodels.Interaction) (string, error) {
	if out := interaction.GetOutputText(); out != nil && *out != "" {
		return *out, nil
	}

	text, found := lastModelOutputText(interaction.GetSteps())
	if text != "" {
		return text, nil
	}

	if !found {
		return "", errors.New("model returned no output text")
	}

	// A model_output step existed but carried no text. Reported as empty so the
	// caller surfaces its own "message is empty" error rather than this.
	return "", nil
}

// lastModelOutputText concatenates the text of the final model_output step,
// which is the one carrying the reply.
func lastModelOutputText(steps []imodels.Step) (string, bool) {
	var last *imodels.ModelOutputStep

	for i := range steps {
		if steps[i].Type != imodels.StepTypeModelOutput || steps[i].ModelOutputStep == nil {
			continue
		}

		last = steps[i].ModelOutputStep
	}

	if last == nil {
		return "", false
	}

	var sb strings.Builder

	for _, content := range last.Content {
		if content.TextContent != nil {
			sb.WriteString(content.TextContent.Text)
		}
	}

	return sb.String(), true
}

// resolveModel picks the model the agent reasons with, preferring the model
// named on the request and falling back to the adapter's configured model. Names
// the agent does not offer — every pre-3.5 Gemini model, for instance — resolve
// to the agent's own default so a stale configuration degrades instead of
// erroring on every call.
func (m *interactionsModel) resolveModel(req *model.LLMRequest) string {
	name := m.name
	if req != nil && req.Model != "" {
		name = req.Model
	}

	return antigravityModel(name)
}

// antigravityModel maps a configured model name onto one the Antigravity agent
// accepts. NewClient applies the same mapping so the model reported to the user
// and written into the Assisted-by trailer is the one that actually served the
// request; a caller comparing the result against the input can see that a name
// outside the agent's set was remapped.
func antigravityModel(configured string) string {
	if slices.Contains(antigravityModels, configured) {
		return configured
	}

	return agentDefaultModel
}

// agentConfig states the model the agent should reason with. It is always set:
// resolveModel has already mapped the incoming name onto an accepted value.
func agentConfig(model string) *imodels.CreateAgentInteractionAgentConfig {
	return genai.Ptr(imodels.NewCreateAgentInteractionAgentConfig(imodels.AntigravityAgentConfig{
		Model: genai.Ptr(model),
	}))
}

// agentEnvironment builds the remote environment, mounting every AGENTS.md the
// caller discovered.
//
// With nothing to mount the bare "remote" string is sent instead, which
// provisions the sandbox unchanged.
//
// Files keep the paths they were given, so a monorepo's tree of nested
// AGENTS.md files is reproduced in the sandbox. The nearest file occupies the
// environment root, which is the slot the Antigravity runtime loads as system
// instructions; the rest sit at their repository-relative paths, where the
// agent can read the one that matches whatever it is looking at.
func agentEnvironment(mounts []AgentFile) imodels.CreateAgentInteractionEnvironment {
	if len(mounts) == 0 {
		return imodels.NewCreateAgentInteractionEnvironment(remoteEnvironment)
	}

	sources := make([]imodels.Source, 0, len(mounts))

	for _, file := range mounts {
		sources = append(sources, imodels.Source{
			Type:    imodels.SourceTypeInline.ToPointer(),
			Target:  genai.Ptr(file.Path),
			Content: genai.Ptr(file.Content),
		})
	}

	return imodels.NewCreateAgentInteractionEnvironment(imodels.Environment{Sources: sources})
}

// promptText flattens request content into the single string the Interactions
// input union accepts, concatenating every text part.
func promptText(contents []*genai.Content) string {
	var sb strings.Builder

	for _, content := range contents {
		if content == nil {
			continue
		}

		for _, part := range content.Parts {
			if part != nil {
				sb.WriteString(part.Text)
			}
		}
	}

	return sb.String()
}
