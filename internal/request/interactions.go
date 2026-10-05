package request

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
	imodels "google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
)

// interactionCreator is the slice of the Interactions API this adapter uses.
// *interactions.Interactions satisfies it; tests substitute a fake so no
// generation reaches the network.
type interactionCreator interface {
	Create(
		ctx context.Context, req operations.CreateInteractionRequest, opts ...operations.Option,
	) (*operations.CreateInteractionResponse, error)
}

// interactionsModel routes commit-message generation through the Gemini
// Interactions API while presenting the agent toolkit's model.LLM interface.
//
// Implementing that interface rather than calling the Interactions API directly
// is deliberate: Client.modelImpl is typed as model.LLM, so every call site,
// every test mock, and the test-only generator constructor are unchanged.
//
// Each generation is a single independent interaction. No previous-interaction
// reference is sent, so no conversation state accumulates across invocations.
type interactionsModel struct {
	name    string
	creator interactionCreator
}

// newInteractionsModel returns a model.LLM backed by the Interactions API.
func newInteractionsModel(name string, creator interactionCreator) *interactionsModel {
	return &interactionsModel{name: name, creator: creator}
}

// Name reports the model this adapter was configured with.
func (m *interactionsModel) Name() string { return m.name }

// GenerateContent performs one interaction and yields exactly one aggregated
// response.
//
// The stream argument is accepted to satisfy model.LLM but does not select
// partial delivery: gud always consumes a single complete message, and
// delivering a half-written commit message would be worse than waiting.
func (m *interactionsModel) GenerateContent(
	ctx context.Context, req *model.LLMRequest, _ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.create(ctx, req)
		if err != nil {
			yield(nil, err)

			return
		}

		yield(resp, nil)
	}
}

// create performs the interaction and converts the result into a model.LLMResponse.
func (m *interactionsModel) create(
	ctx context.Context, req *model.LLMRequest,
) (*model.LLMResponse, error) {
	res, err := m.creator.Create(ctx, operations.CreateInteractionRequest{
		Body: operations.NewCreateInteractionRequestBody(imodels.CreateModelInteraction{
			Model: imodels.Model(m.resolveModel(req)),
			Input: genai.Ptr(imodels.NewInteractionsInput(promptText(req.Contents))),
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
// The message lives in the steps array, on the step whose type is
// "model_output"; thought and tool-call steps carry no user-facing text and are
// skipped. The convenience OutputText field is nil for this response shape, so
// reading only that field silently yields an empty commit message — it is
// consulted afterwards purely as a fallback for response shapes that do
// populate it.
func interactionText(interaction *imodels.Interaction) (string, error) {
	var sb strings.Builder

	found := false

	for _, step := range interaction.GetSteps() {
		if step.Type != imodels.StepTypeModelOutput || step.ModelOutputStep == nil {
			continue
		}

		for _, content := range step.ModelOutputStep.Content {
			if content.TextContent != nil {
				sb.WriteString(content.TextContent.Text)
			}
		}

		found = true
	}

	if sb.Len() > 0 {
		return sb.String(), nil
	}

	if out := interaction.GetOutputText(); out != nil && *out != "" {
		return *out, nil
	}

	if !found {
		return "", errors.New("model returned no output text")
	}

	// A model_output step existed but carried no text. Reported as empty so the
	// caller surfaces its own "message is empty" error rather than this.
	return "", nil
}

// resolveModel prefers the model named on the request, falling back to the
// adapter's configured model so a caller that omits it still reaches the API
// with a valid model.
func (m *interactionsModel) resolveModel(req *model.LLMRequest) string {
	if req != nil && req.Model != "" {
		return req.Model
	}

	return m.name
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
