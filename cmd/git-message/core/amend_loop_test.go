package core

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"iter"
	"os"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"gud/internal/config"
	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// scriptedLLM returns one response per GenerateContent call, repeating the
// last response once the script runs out. A message with a body and no
// path-like tokens produces no verification findings, so each generation is
// exactly one call.
type scriptedLLM struct {
	responses []string
	calls     int
}

func (m *scriptedLLM) Name() string { return "scripted" }

func (m *scriptedLLM) GenerateContent(
	_ context.Context, _ *model.LLMRequest, _ bool,
) iter.Seq2[*model.LLMResponse, error] {
	text := m.responses[min(m.calls, len(m.responses)-1)]
	m.calls++

	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: genai.NewContentFromText(text, "model")}, nil)
	}
}

// amendLoopTestApp builds the minimal AppContext amendReviewLoop needs: a
// resolved config and an offline request client backed by scripted responses.
func amendLoopTestApp(responses ...string) *AppContext {
	return &AppContext{
		cfg:    config.Config{WrapLine: 72},
		client: request.NewClientWithGenerator(&scriptedLLM{responses: responses}, "test-model"),
	}
}

// amendLoopTestJob sets up a two-commit repository and returns the HEAD job.
func amendLoopTestJob(t *testing.T) amendJob {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	dir, run := amendRefusalTestRepo(t)
	run("commit", "-q", "--allow-empty", "-m", "first")

	// HEAD must carry a real change: amending an empty commit would be
	// refused by git unless --allow-empty is repeated.
	if err := os.WriteFile(dir+"/file.txt", []byte("v1\n"), 0600); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}

	run("add", "file.txt")
	run("commit", "-q", "-m", "second")

	ctx := context.Background()

	sha, err := git.ResolveRevision(ctx, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRevision(HEAD): %v", err)
	}

	oldMsg, err := git.GetCommitMessage(ctx, sha)
	if err != nil {
		t.Fatalf("GetCommitMessage(HEAD): %v", err)
	}

	return amendJob{
		sha:    sha,
		diff:   "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n",
		oldMsg: oldMsg,
		head:   true,
	}
}

func runAmendLoop(t *testing.T, app *AppContext, job amendJob, input string) (string, error) {
	t.Helper()

	cmd := &cobra.Command{}

	var out bytes.Buffer

	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	err := amendReviewLoop(context.Background(), cmd, app, bufio.NewScanner(cmd.InOrStdin()), &out, job, "repo context")

	return out.String(), err
}

// TestAmendReviewLoop_Commit pins the default amend path: the generated
// message is finalized and written onto HEAD.
func TestAmendReviewLoop_Commit(t *testing.T) {
	job := amendLoopTestJob(t)
	app := amendLoopTestApp("feat: generated\n\nGenerated body.")

	out, err := runAmendLoop(t, app, job, "y\n")
	if err != nil {
		t.Fatalf("amendReviewLoop() error = %v", err)
	}

	if !strings.Contains(out, "Amended successfully.") {
		t.Errorf("output = %q, want success notice", out)
	}

	msg := mustOutput(t, ".", "log", "-1", "--format=%B")
	if !strings.Contains(msg, "feat: generated") {
		t.Errorf("amended message = %q, want generated message", msg)
	}

	if !strings.Contains(msg, "Assisted-by: test-model") {
		t.Errorf("amended message = %q, want Assisted-by trailer", msg)
	}
}

// TestAmendReviewLoop_Regenerate pins the regenerate loop: the first message
// is discarded and the second generated message is amended.
func TestAmendReviewLoop_Regenerate(t *testing.T) {
	job := amendLoopTestJob(t)
	app := amendLoopTestApp(
		"feat: first\n\nFirst body.",
		"feat: second\n\nSecond body.",
	)

	_, err := runAmendLoop(t, app, job, "r\ny\n")
	if err != nil {
		t.Fatalf("amendReviewLoop() error = %v", err)
	}

	msg := mustOutput(t, ".", "log", "-1", "--format=%B")
	if !strings.Contains(msg, "feat: second") {
		t.Errorf("amended message = %q, want the regenerated message", msg)
	}

	if strings.Contains(msg, "feat: first") {
		t.Errorf("amended message = %q, want the first message discarded", msg)
	}
}

// TestAmendReviewLoop_Edit pins the edit path: the edited message replaces the
// generated one, is finalized, and is written onto HEAD.
func TestAmendReviewLoop_Edit(t *testing.T) {
	editor := writeEditorScript(t, "#!/bin/sh\ncat > \"$1\" << 'EOF'\nfeat: edited\n\nEdited body.\nEOF\n")
	t.Setenv("EDITOR", editor)

	job := amendLoopTestJob(t)
	app := amendLoopTestApp("feat: generated\n\nGenerated body.")

	_, err := runAmendLoop(t, app, job, "e\n")
	if err != nil {
		t.Fatalf("amendReviewLoop() error = %v", err)
	}

	msg := mustOutput(t, ".", "log", "-1", "--format=%B")
	if !strings.Contains(msg, "feat: edited") {
		t.Errorf("amended message = %q, want edited message", msg)
	}

	if !strings.Contains(msg, "Assisted-by: test-model") {
		t.Errorf("amended message = %q, want Assisted-by trailer", msg)
	}
}

// TestAmendReviewLoop_Abort pins the abort path: no commit is rewritten.
func TestAmendReviewLoop_Abort(t *testing.T) {
	job := amendLoopTestJob(t)
	app := amendLoopTestApp("feat: generated\n\nGenerated body.")

	out, err := runAmendLoop(t, app, job, "a\n")
	if err != nil {
		t.Fatalf("amendReviewLoop() error = %v", err)
	}

	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want abort notice", out)
	}

	msg := mustOutput(t, ".", "log", "-1", "--format=%B")
	if strings.Contains(msg, "feat: generated") {
		t.Errorf("amended message = %q, want original message untouched", msg)
	}
}

// TestAmendReviewLoop_NoClient pins the failure path: without a request
// client, generation fails before any commit is rewritten.
func TestAmendReviewLoop_NoClient(t *testing.T) {
	job := amendLoopTestJob(t)
	app := &AppContext{cfg: config.Config{WrapLine: 72}}

	_, err := runAmendLoop(t, app, job, "y\n")
	if err == nil {
		t.Fatal("amendReviewLoop() expected error for a missing client, got nil")
	}

	if !strings.Contains(err.Error(), "request client is unavailable") {
		t.Errorf("error = %v, want request-client failure", err)
	}
}
