// Package tui provides interactive Terminal User Interface components
// using the charmbracelet/bubbletea framework.
package tui

import (
	"context"
	"fmt"
	"strings"

	"gud/internal/profile"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles used across the TUI.
var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA"))
	paginationStyle = list.DefaultStyles().PaginationStyle.PaddingLeft(2)
	helpStyle       = list.DefaultStyles().HelpStyle.PaddingLeft(2).PaddingBottom(1)
	resultStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
	spinnerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C56D9"))
	successStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444"))
)

// catalogItem adapts profile.CatalogEntry for the bubbles list component.
type catalogItem struct {
	entry  profile.CatalogEntry
	cached bool
}

func (i catalogItem) Title() string { return i.entry.Slug }
func (i catalogItem) Description() string {
	var b strings.Builder
	if i.cached {
		b.WriteString("✓ Cached  •  ")
	}
	b.WriteString(i.entry.Profession)
	b.WriteString("  •  ")
	b.WriteString(i.entry.WorkMode)
	if i.entry.Summary != "" {
		b.WriteString("  —  ")
		b.WriteString(truncate(i.entry.Summary, 50))
	}

	return b.String()
}
func (i catalogItem) FilterValue() string {
	return i.entry.Profession + " " + i.entry.Summary + " " + i.entry.WorkMode + " " + i.entry.Slug
}

// DownloadFunc is the signature for the profile download function.
// The TUI calls this when the user selects a profile to save.
type DownloadFunc func(ctx context.Context, slug string) error

// Messages for the picker TUI.
type (
	downloadStartedMsg struct{ slug string }
	downloadDoneMsg    struct{ slug string }
	downloadFailedMsg  struct {
		slug string
		err  error
	}
)

// PickerState represents the current state of the picker.
type PickerState int

const (
	StateBrowsing PickerState = iota
	StateDownloading
	StateDone
	StateFailed
)

// PickerModel is the Bubble Tea model for the profile picker.
type PickerModel struct {
	list     list.Model
	spinner  spinner.Model
	state    PickerState
	entries  []profile.CatalogEntry
	selected *profile.CatalogEntry
	download DownloadFunc
	ctx      context.Context
	err      error
}

// NewPicker creates a new picker model with the given catalog entries,
// download function, and optional cached-slug set and custom title.
// Any slug present in cached will display a cache indicator.
func NewPicker(
	entries []profile.CatalogEntry,
	download DownloadFunc,
	cached map[string]bool,
	title ...string,
) PickerModel {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = catalogItem{entry: e, cached: cached[e.Slug]}
	}

	t := "GUD Profile Catalog"
	if len(title) > 0 && title[0] != "" {
		t = title[0]
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = t
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle
	l.FilterInput.Prompt = "/ "
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	s := spinner.New()
	s.Style = spinnerStyle
	s.Spinner = spinner.Dot

	return PickerModel{
		list:     l,
		spinner:  s,
		state:    StateBrowsing,
		entries:  entries,
		download: download,
		ctx:      context.Background(),
	}
}

// Init initializes the Bubble Tea program.
func (m PickerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.EnterAltScreen)
}

// Update handles messages and events.
func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Subtract 4 lines for the result message area below the list.
		m.list.SetSize(msg.Width, msg.Height-4)

		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case StateBrowsing:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "enter":
				return m.handleSelect()
			}

			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)

			return m, cmd

		case StateDownloading:
			// Ignore keys during download
			return m, nil

		case StateDone, StateFailed:
			return m, tea.Quit
		}

	case spinner.TickMsg:
		if m.state == StateDownloading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)

			return m, cmd
		}

	case downloadStartedMsg:
		m.state = StateDownloading
		m.selected = findEntry(m.entries, msg.slug)

		return m, m.startDownload(msg.slug)

	case downloadDoneMsg:
		m.state = StateDone

		return m, nil

	case downloadFailedMsg:
		m.state = StateFailed
		m.err = msg.err

		return m, nil
	}

	return m, nil
}

// handleSelect processes the user's selection from the list.
func (m PickerModel) handleSelect() (tea.Model, tea.Cmd) {
	i, ok := m.list.SelectedItem().(catalogItem)
	if !ok {
		return m, nil
	}

	return m, func() tea.Msg {
		return downloadStartedMsg{slug: i.entry.Slug}
	}
}

// startDownload returns a tea.Cmd that performs the download if download func is non-nil.
func (m PickerModel) startDownload(slug string) tea.Cmd {
	return func() tea.Msg {
		if m.download != nil {
			if err := m.download(m.ctx, slug); err != nil {
				return downloadFailedMsg{slug: slug, err: err}
			}
		}

		return downloadDoneMsg{slug: slug}
	}
}

// View renders the current state of the picker.
func (m PickerModel) View() string {
	listView := m.list.View()

	switch m.state {
	case StateBrowsing:
		return listView

	case StateDownloading:
		name := "profile"
		if m.selected != nil {
			name = m.selected.Slug
		}

		if m.download == nil {
			return listView + "\n\nSelecting " + name + "..."
		}

		return listView + "\n\n" + m.spinner.View() + " Downloading " + name + "..."

	case StateDone:
		name := "profile"
		if m.selected != nil {
			name = m.selected.Slug
		}

		label := "Saved"
		if m.download == nil {
			label = "Selected"
		}

		return listView + "\n\n" +
			successStyle.Render("✓ "+label+": "+name) +
			"\n" + resultStyle.Render("Press any key to exit.")

	case StateFailed:
		errMsg := "download failed"
		if m.err != nil {
			errMsg = m.err.Error()
		}

		return listView + "\n\n" +
			errorStyle.Render("✗ Error: "+errMsg) +
			"\n" + resultStyle.Render("Press any key to exit.")
	}

	return listView
}

// findEntry looks up a catalog entry by slug.
func findEntry(entries []profile.CatalogEntry, slug string) *profile.CatalogEntry {
	for _, e := range entries {
		if e.Slug == slug {
			return &e
		}
	}

	return nil
}

// RunPicker launches the full-screen profile picker TUI and returns the
// selected catalog entry after successfully downloading it (or selecting it).
// The cached set controls cache indicators on list items.
// Returns nil if the user exited without selecting.
func RunPicker(
	entries []profile.CatalogEntry,
	download DownloadFunc,
	cached map[string]bool,
	title ...string,
) (*profile.CatalogEntry, error) {
	p := tea.NewProgram(
		NewPicker(entries, download, cached, title...),
		tea.WithAltScreen(),
	)

	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tui: %w", err)
	}

	model := result.(PickerModel)
	switch model.state {
	case StateDone:
		return model.selected, nil
	case StateFailed:
		return nil, model.err
	default:
		return nil, nil // user quit without selecting
	}
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return strings.TrimSpace(s[:maxLen-3]) + "..."
}
