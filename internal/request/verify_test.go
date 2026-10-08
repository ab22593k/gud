package request

import (
	"context"
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func findingCodesForTest(fs []Finding) []string {
	codes := make([]string, 0, len(fs))
	for _, f := range fs {
		codes = append(codes, f.Check)
	}
	return codes
}

func TestVerifyMessage(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/main.go b/main.go\n+++ b/main.go\n+package main\n"

	cases := []struct {
		name      string
		msg       string
		wrapLine  int
		wantCodes []string
	}{
		{"clean", "fix: resolve leak\n\nFree buffers on error.", 72, nil},
		{"empty", "  \n ", 72, []string{"empty"}},
		{"long subject", strings.Repeat("x", 73), 72, []string{"subject-length"}},
		{"subject exactly at limit", strings.Repeat("x", 72), 72, nil},
		{"long body ok", "fix: ok\n\n" + strings.Repeat("y", 200), 72, nil},
		{"zero wrap uses default", strings.Repeat("x", 73), 0, []string{"subject-length"}},
		{"fence remnant", "fix: ok\n```\ncode\n```", 72, []string{"fence-remnant"}},
		{"backtick remnant", "fix: touch " + "`main.go`" + " now", 72, []string{"fence-remnant"}},
		{"grounded file", "fix: leak in main.go", 72, nil},
		{"ungrounded file", "fix: leak in other.go", 72, []string{"file-grounding"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := strings.Join(findingCodesForTest(VerifyMessage(tc.msg, diff, tc.wrapLine)), ",")
			want := strings.Join(tc.wantCodes, ",")

			if got != want {
				t.Errorf("VerifyMessage codes = [%s], want [%s]", got, want)
			}
		})
	}
}

// scriptLLM yields canned responses in order, recording call count.
type scriptLLM struct {
	name      string
	responses []string
	calls     int
}

func (m *scriptLLM) Name() string { return m.name }

func (m *scriptLLM) GenerateContent(
	_ context.Context, _ *model.LLMRequest, _ bool,
) iter.Seq2[*model.LLMResponse, error] {
	m.calls++

	text := m.responses[min(m.calls-1, len(m.responses)-1)]

	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: genai.NewContentFromText(text, "model")}, nil)
	}
}

func TestGenerate_RegeneratesOnFindings(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/main.go b/main.go\n+++ b/main.go\n+package main\n"
	llm := &scriptLLM{name: "s", responses: []string{
		"fix: leak in other.go",
		"fix: leak in main.go",
	}}
	client := NewClientWithGenerator(llm, "m")

	got, err := client.GenerateCommitMessageWithContent(context.Background(), diff, "",
		DetailStandard, "", "", defaultWrapLine)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.calls != 2 {
		t.Errorf("calls = %d, want 2 (initial + 1 regeneration)", llm.calls)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("result = %q, want the regenerated grounded message", got)
	}
}

func TestGenerate_ReturnsLastOnPersistentFindings(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/main.go b/main.go\n+++ b/main.go\n+package main\n"
	llm := &scriptLLM{name: "s", responses: []string{"fix: leak in other.go"}}
	client := NewClientWithGenerator(llm, "m")

	got, err := client.GenerateCommitMessageWithContent(context.Background(), diff, "",
		DetailStandard, "", "", defaultWrapLine)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.calls != maxVerifyAttempts {
		t.Errorf("calls = %d, want %d (bounded, then serve last)", llm.calls, maxVerifyAttempts)
	}
	if got == "" {
		t.Error("result is empty, want last output served despite findings")
	}
}
