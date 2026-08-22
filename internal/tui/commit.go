package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Action constants returned by RunCommitReview.
const (
	ActionCommit     = "commit"
	ActionEdit       = "edit"
	ActionRegenerate = "regenerate"
	ActionAbort      = "abort"
)

// Display titles for commit review actions.
const (
	commitTitle     = "✓ Commit"
	editTitle       = "✎ Edit"
	regenerateTitle = "↻ Regenerate"
	abortTitle      = "✗ Abort"
)

var (
	commitKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7C56D9")).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C3C3C"))

	previewBoxStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			PaddingBottom(1)
)

// actionItem adapts commit review actions for the bubbles list component.
type actionItem struct {
	id          string
	title       string
	description string
}

func (i actionItem) Title() string       { return i.title }
func (i actionItem) Description() string { return i.description }
func (i actionItem) FilterValue() string { return i.title + " " + i.description }

func defaultActionItems() []list.Item {
	return []list.Item{
		actionItem{
			id:          ActionCommit,
			title:       commitTitle,
			description: "Accept message and commit staged changes",
		},
		actionItem{
			id:          ActionEdit,
			title:       editTitle,
			description: "Modify commit message inline",
		},
		actionItem{
			id:          ActionRegenerate,
			title:       regenerateTitle,
			description: "Generate a new commit message",
		},
		actionItem{
			id:          ActionAbort,
			title:       abortTitle,
			description: "Cancel without committing",
		},
	}
}

// CommitReviewModel is the Bubble Tea model for reviewing and acting on a
// generated commit message. Supports inline editing via a textarea.
type CommitReviewModel struct {
	list     list.Model
	viewport viewport.Model
	textarea textarea.Model
	msg      string // current message (may be edited)
	action   string
	ready    bool
	editing  bool // true when inline editor is active
	wrapLine int  // max line width for wrapping
}

// NewCommitReview creates a new commit review model.
func NewCommitReview(msg string, wrapLine ...int) CommitReviewModel {
	wl := 72
	if len(wrapLine) > 0 {
		wl = wrapLine[0]
	}

	if wl > 0 {
		msg = wrapText(msg, wl)
	}

	ta := textarea.New()
	ta.SetValue(msg)
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.Placeholder = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Prompt = ""
	ta.Blur()

	items := defaultActionItems()
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Shortcuts"
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return CommitReviewModel{
		msg:      msg,
		textarea: ta,
		list:     l,
		wrapLine: wl,
	}
}

// Init initializes the Bubble Tea program.
func (m CommitReviewModel) Init() tea.Cmd {
	return tea.EnterAltScreen
}

// Update handles messages and events.
func (m CommitReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve height for the preview box (~6 lines) and list (~10 lines).
		listHeight := max(min(10, msg.Height-6), 5)

		vpHeight := max(msg.Height-listHeight-6, 3)

		vpWidth := msg.Width - 6
		if m.wrapLine > 0 && m.wrapLine < vpWidth {
			vpWidth = m.wrapLine
		}

		taWidth := msg.Width - 4
		if m.wrapLine > 0 && m.wrapLine+4 < taWidth {
			taWidth = m.wrapLine + 4
		}

		if !m.ready {
			m.viewport = viewport.New(vpWidth, vpHeight)
			m.viewport.SetContent(m.msg)
			m.viewport.YPosition = 0

			m.list.SetSize(msg.Width, listHeight)

			m.textarea.SetWidth(taWidth)
			m.textarea.SetHeight(msg.Height - 6)
			m.ready = true
		} else {
			m.viewport.Width = vpWidth
			m.viewport.Height = vpHeight
			m.list.SetSize(msg.Width, listHeight)
			m.textarea.SetWidth(taWidth)
			m.textarea.SetHeight(msg.Height - 6)
		}

		return m, nil

	case tea.KeyMsg:
		switch {
		case m.editing:
			return m.handleEditKey(msg)
		default:
			return m.handleReviewKey(msg)
		}
	}

	return m, nil
}

// handleReviewKey processes keys in review mode (list navigation + shortcut keys).
func (m CommitReviewModel) handleReviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "c", "C":
		m.action = ActionCommit

		return m, tea.Quit

	case "r", "R":
		m.action = ActionRegenerate

		return m, tea.Quit

	case "e", "E":
		m.editing = true
		m.textarea.SetValue(m.msg)
		m.textarea.Focus()

		return m, textarea.Blink

	case "a", "A", "q", "Q", "ctrl+c":
		m.action = ActionAbort

		return m, tea.Quit

	case "enter":
		if sel, ok := m.list.SelectedItem().(actionItem); ok {
			switch sel.id {
			case ActionCommit:
				m.action = ActionCommit

				return m, tea.Quit
			case ActionEdit:
				m.editing = true
				m.textarea.SetValue(m.msg)
				m.textarea.Focus()

				return m, textarea.Blink
			case ActionRegenerate:
				m.action = ActionRegenerate

				return m, tea.Quit
			case ActionAbort:
				m.action = ActionAbort

				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

// handleEditKey processes keys in inline edit mode.
func (m CommitReviewModel) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s", "alt+enter":
		// Confirm edit — apply wrapLine, set content, and return to review.
		// alt+enter is a second confirm path: some terminals swallow ctrl+s
		// (XOFF flow control), which would otherwise strand the user in edit
		// mode with their changes unrecoverable.
		edited := m.textarea.Value()
		if m.wrapLine > 0 {
			edited = wrapText(edited, m.wrapLine)
		}
		m.msg = edited
		m.viewport.SetContent(m.msg)
		m.editing = false
		m.textarea.Blur()

		return m, nil

	case "esc":
		// Cancel edit — restore the last confirmed message (discards the draft).
		m.textarea.SetValue(m.msg)
		m.editing = false
		m.textarea.Blur()

		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	return m, cmd
}

// View renders the current state.
func (m CommitReviewModel) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	if m.editing {
		return m.editView()
	}

	return m.reviewView()
}

// reviewView renders the review mode (preview box + list of actions).
func (m CommitReviewModel) reviewView() string {
	previewHeader := titleStyle.Render("Generated Commit Message:\n")
	previewContent := previewBoxStyle.Render(m.viewport.View())

	return fmt.Sprintf("%s\n%s\n\n%s", previewHeader, previewContent, m.list.View())
}

// editView renders the inline editor mode (textarea + edit help bar).
func (m CommitReviewModel) editView() string {
	header := titleStyle.Render("Edit Commit Message")
	sep := separatorStyle.Render(strings.Repeat("─", m.textarea.Width()))
	help := helpStyle.Render(
		commitKeyStyle.Render("ctrl+s") + "/" + commitKeyStyle.Render("alt+enter") + " confirm  " +
			commitKeyStyle.Render("esc") + " cancel",
	)

	return fmt.Sprintf("%s\n%s\n%s\n%s", header, sep, m.textarea.View(), help)
}

// wrapText wraps lines in text to max character width `limit`.
// Lines that are already shorter than `limit` are left unchanged.
func wrapText(text string, limit int) string {
	if limit <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if len(line) <= limit {
			result = append(result, line)

			continue
		}

		// Preserve leading indentation (spaces or tabs)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, line)

			continue
		}

		var currentLine strings.Builder
		currentLine.WriteString(indent)
		currentLine.WriteString(words[0])

		for _, word := range words[1:] {
			if currentLine.Len()+1+len(word) > limit {
				result = append(result, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(indent)
				currentLine.WriteString(word)
			} else {
				currentLine.WriteString(" ")
				currentLine.WriteString(word)
			}
		}
		if currentLine.Len() > 0 {
			result = append(result, currentLine.String())
		}
	}

	return strings.Join(result, "\n")
}

// RunCommitReview launches the full-screen commit review TUI and returns the
// user's chosen action: ActionCommit, ActionRegenerate, or ActionAbort.
// Editing is handled inline within the TUI — when the user selects 'Edit' or presses 'e',
// a textarea replaces the view for in-place editing. After confirming
// with Ctrl+S, the edited message is shown in the preview box for review
// before the user picks a final action.
// The optional wrapLine parameter specifies line width wrapping for text.
// The second return value is the (possibly edited) message from the TUI.
func RunCommitReview(msg string, wrapLine ...int) (string, string, error) {
	p := tea.NewProgram(NewCommitReview(msg, wrapLine...), tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return ActionAbort, msg, err
	}

	model, ok := result.(CommitReviewModel)
	if !ok {
		return ActionAbort, msg, fmt.Errorf("tui: unexpected model type %T", result)
	}
	if model.action == "" {
		return ActionAbort, msg, nil
	}

	return model.action, model.msg, nil
}
