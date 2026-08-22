package tui

import (
	"context"
	"errors"
	"testing"

	"gud/internal/profile"

	tea "github.com/charmbracelet/bubbletea"
)

func testEntries() []profile.CatalogEntry {
	return []profile.CatalogEntry{
		{Slug: "plumber", Profession: "Plumber", WorkMode: "work", Summary: "fixes pipes"},
		{Slug: "dev-ops", Profession: "DevOps Engineer", WorkMode: "work", Summary: "ships"},
	}
}

func TestFindEntry(t *testing.T) {
	t.Parallel()

	entries := testEntries()
	if got := findEntry(entries, "plumber"); got == nil || got.Slug != "plumber" {
		t.Errorf("findEntry(plumber) = %v, want plumber entry", got)
	}
	if got := findEntry(entries, "missing"); got != nil {
		t.Errorf("findEntry(missing) = %v, want nil", got)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{name: "short string unchanged", s: "abc", maxLen: 10, want: "abc"},
		{name: "exact length unchanged", s: "abcdefghij", maxLen: 10, want: "abcdefghij"},
		{name: "long string truncated", s: "abcdefghijklmno", maxLen: 10, want: "abcdefg..."},
		{name: "trailing space trimmed", s: "abcdefghijk ", maxLen: 10, want: "abcdefg..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := truncate(tt.s, tt.maxLen); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestCatalogItemDescription(t *testing.T) {
	t.Parallel()

	item := catalogItem{entry: profile.CatalogEntry{
		Slug: "p", Profession: "Plumber", WorkMode: "work", Summary: "fixes pipes",
	}}
	if got, want := item.Description(), "Plumber  •  work  —  fixes pipes"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}

	cached := catalogItem{entry: item.entry, cached: true}
	if got := cached.Description(); got[:len("✓ Cached")] != "✓ Cached" {
		t.Errorf("cached Description() = %q, want cache indicator prefix", got)
	}
}

func newTestPicker(t *testing.T) PickerModel {
	t.Helper()

	return NewPicker(testEntries(), nil, map[string]bool{"plumber": true})
}

func TestPickerUpdate_QuitKey(t *testing.T) {
	t.Parallel()

	m := newTestPicker(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("Update(q) cmd = nil, want quit command")
	}
}

func TestPickerUpdate_WindowSize(t *testing.T) {
	t.Parallel()

	m := newTestPicker(t)
	updated, sizeCmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if sizeCmd != nil {
		t.Errorf("WindowSizeMsg cmd = %v, want nil", sizeCmd)
	}
	pm := updated.(PickerModel)
	if w := pm.list.Width(); w != 80 {
		t.Errorf("list width = %d, want 80", w)
	}
}

func TestPickerUpdate_EnterSelects(t *testing.T) {
	t.Parallel()

	m := newTestPicker(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter cmd = nil, want selection command")
	}
	msg := cmd()
	started, ok := msg.(downloadStartedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want downloadStartedMsg", msg)
	}
	if started.slug == "" {
		t.Error("downloadStartedMsg.slug is empty")
	}
}

func TestPickerUpdate_DownloadLifecycle(t *testing.T) {
	t.Parallel()

	m := newTestPicker(t)

	// downloadStartedMsg moves to downloading and kicks off the download cmd.
	updated, _ := m.Update(downloadStartedMsg{slug: "plumber"})
	pm := updated.(PickerModel)
	if pm.state != StateDownloading {
		t.Errorf("state after started = %v, want StateDownloading", pm.state)
	}
	if pm.selected == nil || pm.selected.Slug != "plumber" {
		t.Errorf("selected = %v, want plumber entry", pm.selected)
	}

	// Keys are ignored while downloading.
	_, keyCmd := pm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if keyCmd != nil {
		t.Errorf("key during download produced cmd %v, want nil", keyCmd)
	}

	// Success transitions to done.
	done, _ := pm.Update(downloadDoneMsg{slug: "plumber"})
	if done.(PickerModel).state != StateDone {
		t.Errorf("state after done = %v, want StateDone", done.(PickerModel).state)
	}

	// Failure records the error.
	failed, _ := pm.Update(downloadFailedMsg{slug: "plumber", err: errors.New("boom")})
	fm := failed.(PickerModel)
	if fm.state != StateFailed || fm.err == nil {
		t.Errorf("state = %v, err = %v; want StateFailed with error", fm.state, fm.err)
	}
}

func TestPickerStartDownload_Cmd(t *testing.T) {
	t.Parallel()

	var called string
	m := NewPicker(testEntries(), func(_ context.Context, slug string) error {
		called = slug

		return errors.New("download boom")
	}, nil)

	msg := m.startDownload("plumber")()
	failed, ok := msg.(downloadFailedMsg)
	if !ok {
		t.Fatalf("startDownload cmd() = %T, want downloadFailedMsg", msg)
	}
	if called != "plumber" || failed.err == nil {
		t.Errorf("called=%q err=%v; want plumber invoked and error propagated", called, failed.err)
	}
}

func TestPickerStateTransition_DoneQuitsOnAnyKey(t *testing.T) {
	t.Parallel()

	m := newTestPicker(t)
	done, _ := m.Update(downloadDoneMsg{slug: "plumber"})
	dm := done.(PickerModel)
	_, cmd := dm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("key in StateDone cmd = nil, want quit command")
	}
}
