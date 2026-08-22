package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewCommitReview(t *testing.T) {
	msg := "feat: add super cool feature"
	model := NewCommitReview(msg)

	if model.msg != msg {
		t.Errorf("expected msg %q, got %q", msg, model.msg)
	}

	if len(model.list.Items()) != 4 {
		t.Errorf("expected 4 action items, got %d", len(model.list.Items()))
	}
}

func TestCommitReviewUpdateWindowSize(t *testing.T) {
	model := NewCommitReview("feat: add feature")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, ok := updated.(CommitReviewModel)
	if !ok {
		t.Fatalf("updated model type = %T, want CommitReviewModel", updated)
	}

	if !m.ready {
		t.Errorf("expected model to be ready after WindowSizeMsg")
	}

	view := m.View()
	if view == "" {
		t.Errorf("expected view to render non-empty string")
	}
}

func TestCommitReviewKeyActions(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		expectedAction string
		expectedEdit   bool
	}{
		{
			name:           "y key commits",
			key:            "y",
			expectedAction: ActionCommit,
			expectedEdit:   false,
		},
		{
			name:           "r key regenerates",
			key:            "r",
			expectedAction: ActionRegenerate,
			expectedEdit:   false,
		},
		{
			name:           "a key aborts",
			key:            "a",
			expectedAction: ActionAbort,
			expectedEdit:   false,
		},
		{
			name:           "e key triggers editing",
			key:            "e",
			expectedAction: "",
			expectedEdit:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewCommitReview("test message")
			model.ready = true
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})

			m, ok := updated.(CommitReviewModel)
			if !ok {
				t.Fatalf("updated model type = %T, want CommitReviewModel", updated)
			}

			if m.action != tt.expectedAction {
				t.Errorf("expected action %q, got %q", tt.expectedAction, m.action)
			}

			if m.editing != tt.expectedEdit {
				t.Errorf("expected editing %v, got %v", tt.expectedEdit, m.editing)
			}
		})
	}
}

func TestCommitReviewEnterSelection(t *testing.T) {
	model := NewCommitReview("test message")
	model.ready = true
	// By default item 0 is Commit ("✓ Commit")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m, ok := updated.(CommitReviewModel)
	if !ok {
		t.Fatalf("updated model type = %T, want CommitReviewModel", updated)
	}

	if m.action != ActionCommit {
		t.Errorf("expected action %q, got %q", ActionCommit, m.action)
	}

	if cmd == nil {
		t.Errorf("expected quit cmd on select enter")
	}
}

func TestActionItemProperties(t *testing.T) {
	item := actionItem{
		id:          ActionCommit,
		title:       commitTitle,
		description: "Accept message",
	}

	if item.Title() != commitTitle {
		t.Errorf("unexpected title: %s", item.Title())
	}

	if item.Description() != "Accept message" {
		t.Errorf("unexpected description: %s", item.Description())
	}

	if item.FilterValue() != "✓ Commit Accept message" {
		t.Errorf("unexpected filter value: %s", item.FilterValue())
	}
}

func TestWrapText(t *testing.T) {
	longLine := "This is a long commit message paragraph that exceeds forty characters."
	wrapped := wrapText(longLine, 40)

	lines := strings.SplitSeq(wrapped, "\n")
	for l := range lines {
		if len(l) > 40 {
			t.Errorf("line length %d exceeds max wrap limit 40: %q", len(l), l)
		}
	}
}

func TestCommitReviewRespectsWrapLine(t *testing.T) {
	longMsg := "This is a very long commit message line that exceeds fifty characters and should be wrapped in the TUI."
	model := NewCommitReview(longMsg, 50)

	if model.wrapLine != 50 {
		t.Errorf("expected wrapLine 50, got %d", model.wrapLine)
	}

	for line := range strings.SplitSeq(model.msg, "\n") {
		if len(line) > 50 {
			t.Errorf("line in model.msg exceeds wrapLine 50: %q", line)
		}
	}

	// Test WindowSizeMsg bounds viewport to wrapLine
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	m, ok := updated.(CommitReviewModel)
	if !ok {
		t.Fatalf("updated model type = %T, want CommitReviewModel", updated)
	}

	if m.viewport.Width > 50 {
		t.Errorf("expected viewport width <= 50, got %d", m.viewport.Width)
	}
}

// TestCommitReviewEditConfirmPreservesEdits guards the edit → commit flow:
// the text typed in the inline editor must survive confirm (ctrl+s or
// alt+enter) and end up in the message returned by the model. Regresses
// "edits lost after entering the TUI edit phase".
func TestCommitReviewEditConfirmPreservesEdits(t *testing.T) {
	for _, confirmKey := range []struct {
		msg tea.KeyMsg
	}{{msg: tea.KeyMsg{Type: tea.KeyCtrlS}}, {msg: tea.KeyMsg{Type: tea.KeyEnter, Alt: true}}} {
		t.Run(confirmKey.msg.String(), func(t *testing.T) {
			m := NewCommitReview("fix: hello world", 0)
			m.ready = true

			// Try to reproduce the user-visible sequence from the report:
			// e (edit) → ctrl+n (cursor to end) → type → confirm → y (commit).
			keys := []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("e")},
				{Type: tea.KeyCtrlN},
				{Type: tea.KeyRunes, Runes: []rune(" touched")},
				confirmKey.msg,
				{Type: tea.KeyRunes, Runes: []rune("y")},
			}

			for _, k := range keys {
				updated, _ := m.Update(k)

				var cok bool
				if m, cok = updated.(CommitReviewModel); !cok {
					t.Fatalf("updated model type = %T, want CommitReviewModel", updated)
				}
			}

			if m.action != ActionCommit {
				t.Errorf("action = %q, want %q", m.action, ActionCommit)
			}

			if !strings.Contains(m.msg, "touched") {
				t.Errorf("edited message not preserved: msg = %q, want it to contain %q", m.msg, "touched")
			}
		})
	}
}

// TestCommitReviewProgramLoopPreservesEdits runs the full Bubble Tea program
// over a raw key stream (edit → type → confirm → commit) and asserts the
// program's final model still carries the edit. This covers the message loop
// and renderer paths that pure model tests cannot.
func TestCommitReviewProgramEditPreservesEdits(t *testing.T) {
	var input bytes.Buffer
	input.WriteString("e")
	input.WriteByte(0x0e) // ctrl+n: cursor to end
	input.WriteString(" touched")
	input.WriteByte(0x13)  // ctrl+s: confirm
	input.WriteString("y") // commit, which quits the program

	p := tea.NewProgram(
		NewCommitReview("fix: hello world", 0),
		tea.WithInput(&input),
		tea.WithOutput(io.Discard),
	)

	result, err := p.Run()
	if err != nil {
		t.Fatalf("program error: %v", err)
	}

	m, rok := result.(CommitReviewModel)
	if !rok {
		t.Fatalf("result model type = %T, want CommitReviewModel", result)
	}

	if !strings.Contains(m.msg, "touched") {
		t.Errorf("edited message not preserved through program loop: msg = %q", m.msg)
	}
}
