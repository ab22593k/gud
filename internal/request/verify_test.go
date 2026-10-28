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

	// cutDiff is a rename whose delete half was truncated away: it carries
	// no rename record and no mention of the pre-image, so a description
	// naming old_name.go is ungrounded unless the probe paths supply it.
	cutDiff := "diff --git a/new_name.go b/new_name.go\n" +
		"new file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/new_name.go\n" +
		"@@ -0,0 +1 @@\n+package new\n"

	cases := []struct {
		name      string
		msg       string
		diff      string
		paths     []string
		wrapLine  int
		wantCodes []string
	}{
		{"clean", "fix: resolve leak\n\nFree buffers on error.", diff, nil, 72, nil},
		{"empty", "  \n ", diff, nil, 72, []string{"empty"}},
		{"subject only", "fix: resolve leak", diff, nil, 72, []string{"empty-body"}},
		{"no blank separator", "fix: resolve leak\nFree buffers on error.", diff, nil, 72, []string{"empty-body"}},
		{"blank-lines-only body", "fix: resolve leak\n\n   \n\t\n", diff, nil, 72, []string{"empty-body"}},
		{"long subject", strings.Repeat("x", 73), diff, nil, 72, []string{"empty-body", "subject-length"}},
		{"subject exactly at limit", strings.Repeat("x", 72), diff, nil, 72, []string{"empty-body"}},
		{"cjk subject within cell limit", strings.Repeat("漢", 25), diff, nil, 72, []string{"empty-body"}},
		{"cjk subject over cell limit", strings.Repeat("漢", 37), diff, nil, 72, []string{"empty-body", "subject-length"}},
		{"emoji subject within cell limit", strings.Repeat("🦀", 36), diff, nil, 72, []string{"empty-body"}},
		{"long body ok", "fix: ok\n\n" + strings.Repeat("y", 200), diff, nil, 72, nil},
		{"zero wrap uses default", strings.Repeat("x", 73), diff, nil, 0, []string{"empty-body", "subject-length"}},
		{"fence remnant", "fix: ok\n```\ncode\n```", diff, nil, 72, []string{"empty-body", "fence-remnant"}},
		{"backtick remnant", "fix: touch " + "`main.go`" + " now", diff, nil, 72, []string{"empty-body", "fence-remnant"}},
		{"grounded file", "fix: leak in main.go", diff, nil, 72, []string{"empty-body"}},
		{"ungrounded file", "fix: leak in other.go", diff, nil, 72, []string{"empty-body", "file-grounding"}},
		// D9 union cases: the probe paths widen the allowed set without
		// removing anything the diff already grounds.
		{
			"rename pre-image grounded by probe", "fix: rename old_name.go\n\nExplain the rename.",
			cutDiff, []string{"old_name.go", "new_name.go"}, 72, nil,
		},
		{
			"both sides of a copy record pass", "fix: copy src.go to dst.go\n\nExplain the copy.",
			diff, []string{"src.go", "dst.go"}, 72, nil,
		},
		{
			"deleted file present only in paths", "fix: remove gone.go\n\nExplain the removal.",
			diff, []string{"gone.go"}, 72, nil,
		},
		{
			"basename of a probe path passes", "fix: touch deep.go\n\nExplain the touch.",
			diff, []string{"sub/deep.go"}, 72, nil,
		},
		{
			"diff-only path passes when paths omits it", "fix: leak in main.go\n\nExplain the fix.",
			diff, []string{"unrelated.go"}, 72, nil,
		},
		{
			"nil paths reproduces diff-only grounding", "fix: leak in other.go\n\nExplain the fix.",
			diff, nil, 72, []string{"file-grounding"},
		},
		{
			"probe path grounds the same mention", "fix: leak in other.go\n\nExplain the fix.",
			diff, []string{"other.go"}, 72, nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			change := Change{Diff: tc.diff, Paths: tc.paths}
			got := strings.Join(findingCodesForTest(VerifyMessage(tc.msg, change, tc.wrapLine)), ",")
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

	change := Change{Diff: diff}
	if got := findingCodesForTest(VerifyMessage("fix: leak in a.txt\n\nExplain the fix.", change, 72)); len(got) != 0 {
		t.Errorf("VerifyMessage codes = %v, want none (basename grounded, body present)", got)
	}
}

// TestVerifyGroundingSubstringBoundary pins the intentional tightening:
// "domain.go" merely containing "main.go" as a substring no longer grounds
// a mention of main.go, which names a different file.
func TestVerifyGroundingSubstringBoundary(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/domain.go b/domain.go\n" +
		"index 1111111..2222222 100644\n--- a/domain.go\n+++ b/domain.go\n"

	got := findingCodesForTest(VerifyMessage("fix: leak in main.go", Change{Diff: diff}, 72))
	if len(got) != 2 || got[0] != "empty-body" || got[1] != "file-grounding" {
		t.Errorf("VerifyMessage codes = %v, want [empty-body file-grounding]", got)
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

	got := findingCodesForTest(VerifyMessage(sb.String(), Change{Diff: diff}, 72))

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

// scriptLLM yields canned responses in order, recording call count and the
// text of every request it receives (so tests can assert what the bounded
// retry actually sent).
type scriptLLM struct {
	name      string
	responses []string
	calls     int
	requests  []string
}

func (m *scriptLLM) Name() string { return m.name }

func (m *scriptLLM) GenerateContent(
	_ context.Context, req *model.LLMRequest, _ bool,
) iter.Seq2[*model.LLMResponse, error] {
	m.calls++

	for _, c := range req.Contents {
		if text, err := extractText(c); err == nil {
			m.requests = append(m.requests, text)
		}
	}

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

	got, _, err := client.GenerateCommitMessageWithContent(context.Background(), Change{Diff: diff}, "",
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

	got, _, err := client.GenerateCommitMessageWithContent(context.Background(), Change{Diff: diff}, "",
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

// TestGenerate_PersistsBodyLessFinding pins FR-001 through the generation
// loop: a reply that is grounded, short, and fence-free but has no body must
// still be regenerated up to the bound, and the surviving empty-body finding
// must be returned so the caller can present it instead of a clean result.
func TestGenerate_PersistsBodyLessFinding(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/main.go b/main.go\n+++ b/main.go\n+package main\n"
	llm := &scriptLLM{name: "s", responses: []string{"fix: leak in main.go"}}
	client := NewClientWithGenerator(llm, "m")

	got, findings, err := client.GenerateCommitMessageWithContent(context.Background(), Change{Diff: diff}, "",
		DetailStandard, "", nil, defaultWrapLine)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.calls != maxVerifyAttempts {
		t.Errorf("calls = %d, want %d (body-less retried to the bound)", llm.calls, maxVerifyAttempts)
	}
	if got == "" {
		t.Error("result is empty, want last output served despite findings")
	}
	if codes := findingCodesForTest(findings); len(codes) != 1 || codes[0] != "empty-body" {
		t.Errorf("residual findings = %v, want [empty-body]", codes)
	}
}

// TestGenerateCommitMessageWithContent_CarriesChange pins the Change
// boundary through the generator: a scripted reply naming only the rename
// pre-image is clean when Change.Paths reports the rename, and yields
// file-grounding when the same diff is passed with Paths: nil. Do not
// duplicate the verifier cases owned by TestVerifyMessage.
func TestGenerateCommitMessageWithContent_CarriesChange(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/new_name.go b/new_name.go\n" +
		"new file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/new_name.go\n" +
		"@@ -0,0 +1 @@\n+package new\n"
	paths := []string{"old_name.go", "new_name.go"}
	msg := "Rename old_name.go\n\nRename old_name.go to new_name.go."

	withPaths := &scriptLLM{name: "s", responses: []string{msg}}
	withClient := NewClientWithGenerator(withPaths, "m")

	got, findings, err := withClient.GenerateCommitMessageWithContent(context.Background(),
		Change{Diff: diff, Paths: paths}, "", DetailStandard, "", nil, defaultWrapLine)
	if err != nil {
		t.Fatalf("Generate(paths) = %v", err)
	}

	if got == "" {
		t.Error("Generate(paths) is empty, want the served reply")
	}

	if codes := findingCodesForTest(findings); len(codes) != 0 {
		t.Errorf("Generate(paths) findings = %v, want none (pre-image grounded by Paths)", codes)
	}

	withoutPaths := &scriptLLM{name: "s", responses: []string{msg}}
	withoutClient := NewClientWithGenerator(withoutPaths, "m")

	_, findings, err = withoutClient.GenerateCommitMessageWithContent(context.Background(),
		Change{Diff: diff}, "", DetailStandard, "", nil, defaultWrapLine)
	if err != nil {
		t.Fatalf("Generate(nil paths) = %v", err)
	}

	if codes := findingCodesForTest(findings); len(codes) != 1 || codes[0] != "file-grounding" {
		t.Errorf("Generate(nil paths) findings = %v, want [file-grounding]", codes)
	}
}

// TestGenerate_FeedsFindingsBack pins FR-018 at the model boundary: when the
// first reply fails a check, the bounded retry must not re-send the same
// request — it carries a corrective block naming the failed check and its
// detail, and instructs the model to output only the corrected message. A
// retry that converges returns no residual findings.
func TestGenerate_FeedsFindingsBack(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/main.go b/main.go\n+++ b/main.go\n+package main\n"
	llm := &scriptLLM{name: "s", responses: []string{
		"fix: leak in other.go",
		"fix: leak in main.go\n\nFree buffers on error.",
	}}
	client := NewClientWithGenerator(llm, "m")

	got, findings, err := client.GenerateCommitMessageWithContent(context.Background(), Change{Diff: diff}, "",
		DetailStandard, "", nil, defaultWrapLine)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.calls != 2 {
		t.Fatalf("calls = %d, want 2 (initial + 1 corrective retry)", llm.calls)
	}
	if len(findings) != 0 {
		t.Errorf("residual findings = %v, want none (retry converged)", findingCodesForTest(findings))
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("result = %q, want the corrected grounded message", got)
	}

	if len(llm.requests) != 2 {
		t.Fatalf("captured requests = %d, want 2", len(llm.requests))
	}

	retry := llm.requests[1]
	if !strings.Contains(retry, "file-grounding") {
		t.Errorf("retry request does not name the failed check:\n%q", retry)
	}
	if !strings.Contains(retry, "other.go") {
		t.Errorf("retry request does not carry the finding detail:\n%q", retry)
	}
	if !strings.Contains(retry, "corrected message") {
		t.Errorf("retry request does not instruct an output-only correction:\n%q", retry)
	}
}
