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
		{"cjk subject within cell limit", strings.Repeat("漢", 25), 72, nil},
		{"cjk subject over cell limit", strings.Repeat("漢", 37), 72, []string{"subject-length"}},
		{"emoji subject within cell limit", strings.Repeat("🦀", 36), 72, nil},
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

// TestVerifyGroundingBasenameMention guards a behavior the substring scan
// provided for free: a bare basename ("fix a.txt") is grounded by diff path
// "sub/a.txt". The token-set index keeps this via basename entries.
func TestVerifyGroundingBasenameMention(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/sub/a.txt b/sub/a.txt\n" +
		"index 1111111..2222222 100644\n--- a/sub/a.txt\n+++ b/sub/a.txt\n"

	if got := findingCodesForTest(VerifyMessage("fix: leak in a.txt", diff, 72)); len(got) != 0 {
		t.Errorf("VerifyMessage codes = %v, want none (basename grounded)", got)
	}
}

// TestVerifyGroundingSubstringBoundary pins the intentional tightening:
// "domain.go" merely containing "main.go" as a substring no longer grounds
// a mention of main.go, which names a different file.
func TestVerifyGroundingSubstringBoundary(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/domain.go b/domain.go\n" +
		"index 1111111..2222222 100644\n--- a/domain.go\n+++ b/domain.go\n"

	got := findingCodesForTest(VerifyMessage("fix: leak in main.go", diff, 72))
	if len(got) != 1 || got[0] != "file-grounding" {
		t.Errorf("VerifyMessage codes = %v, want [file-grounding]", got)
	}
}

// TestVerifyGroundingLargeDiff exercises the scale path deterministically: a
// ~150KB diff with 200 ungrounded tokens plus one grounded control. No timing
// assertion (load variance); linearity is by construction — one diff pass
// plus map lookups — and allocation counts are covered by the benchmarks.
func TestVerifyGroundingLargeDiff(t *testing.T) {
	t.Parallel()

	diff := benchDiff(50, 30)

	var sb strings.Builder

	sb.WriteString("Refactor\n\nTouch pkg/module00/file000.go and ")

	for f := range 200 {
		sb.WriteString("ghost")
		sb.WriteString(itoa3(f))
		sb.WriteString(".go ")
	}

	got := findingCodesForTest(VerifyMessage(sb.String(), diff, 72))

	grounded := 0

	for _, f := range got {
		if f != "file-grounding" {
			t.Fatalf("VerifyMessage codes = %v, want only file-grounding", got)
		}

		grounded++
	}

	if grounded != 200 {
		t.Errorf("findings = %d, want 200 (ghosts only; the real path stays grounded)", grounded)
	}
}

// TestIndexDiffTokensMatchesPathToken locks the linear scanner to the regex
// it replaces: over crafted shapes and a real diff, both must report the
// same token set. Inputs stay small so the regex side is cheap.
func TestIndexDiffTokensMatchesPathToken(t *testing.T) {
	t.Parallel()

	shapes := []string{
		"a.b", "a.b/c", ".foo", "a.b/-c", "Xmain.go main.go", "a.b/c.d",
		`"b/my file.go"`, "héllo.go", "a...b", "--- a/old.txt", "+++ b/new.go",
		"v1.2.3", "$VAR", "a/b/c/d/e.go trace.go,",
		benchDiff(2, 3),
	}

	for _, s := range shapes {
		want := map[string]bool{}
		for _, tok := range pathToken.FindAllString(s, -1) {
			want[tok] = true
		}

		got := map[string]bool{}
		indexDiffTokens(got, s)

		// indexDiffTokens also adds basenames; strip those before comparing.
		for tok := range got {
			if !want[tok] {
				delete(got, tok)
			}
		}

		if len(got) != len(want) {
			t.Errorf("tokens(%q): index=%v want=%v", s, got, want)
		}
	}
}

func itoa3(n int) string {
	s := ""
	for range 3 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}

	return s
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
		DetailStandard, "", nil, defaultWrapLine)
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
		DetailStandard, "", nil, defaultWrapLine)
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
