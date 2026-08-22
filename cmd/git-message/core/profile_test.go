package core

import (
	"testing"

	"gud/internal/profile"
)

const (
	testBiology = "biology"
	testPhysics = "physics"
)

func TestCategorizeByWorkMode(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		input []profile.CatalogEntry
		want  []category
	}

	tests := []testCase{
		{
			name:  "nil input returns empty slice",
			input: nil,
			want:  []category{},
		},
		{
			name:  "empty input returns empty slice",
			input: []profile.CatalogEntry{},
			want:  []category{},
		},
		{
			name: "single entry",
			input: []profile.CatalogEntry{
				{WorkMode: testPhysics, Profession: testAstrophysicist},
			},
			want: []category{
				{name: testPhysics, count: 1},
			},
		},
		{
			name: "multiple entries same work mode",
			input: []profile.CatalogEntry{
				{WorkMode: testPhysics, Profession: testAstrophysicist},
				{WorkMode: testPhysics, Profession: "cosmologist"},
				{WorkMode: testPhysics, Profession: "quantum physicist"},
			},
			want: []category{
				{name: testPhysics, count: 3},
			},
		},
		{
			name: "entries grouped by work mode and sorted by name",
			input: []profile.CatalogEntry{
				{WorkMode: testBiology, Profession: "molecular biologist"},
				{WorkMode: testPhysics, Profession: testAstrophysicist},
				{WorkMode: "chemistry", Profession: "organic chemist"},
				{WorkMode: testBiology, Profession: "geneticist"},
			},
			want: []category{
				{name: testBiology, count: 2},
				{name: "chemistry", count: 1},
				{name: testPhysics, count: 1},
			},
		},
		{
			name: "single element in each work mode",
			input: []profile.CatalogEntry{
				{WorkMode: "z", Profession: "last"},
				{WorkMode: "a", Profession: "first"},
				{WorkMode: "m", Profession: "middle"},
			},
			want: []category{
				{name: "a", count: 1},
				{name: "m", count: 1},
				{name: "z", count: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := categorizeByWorkMode(tt.input)

			if len(got) != len(tt.want) {
				t.Fatalf("categorizeByWorkMode() returned %d categories, want %d\ngot:  %+v\nwant: %+v",
					len(got), len(tt.want), got, tt.want)
			}

			for i := range got {
				if got[i].name != tt.want[i].name || got[i].count != tt.want[i].count {
					t.Errorf("categorizeByWorkMode()[%d] = {name:%q, count:%d}, want {name:%q, count:%d}",
						i, got[i].name, got[i].count, tt.want[i].name, tt.want[i].count)
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	const (
		hello      = "hello"
		helloWorld = "hello world"
	)

	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{name: "shorter than max returns as-is", s: hello, maxLen: 10, want: hello},
		{name: "equal to max returns as-is", s: hello, maxLen: 5, want: hello},
		{name: "longer than max appends ellipsis", s: helloWorld, maxLen: 5, want: "hello..."},
		{name: "empty string returns empty", s: "", maxLen: 10, want: ""},
		{name: "maxLen of zero with content", s: hello, maxLen: 0, want: "..."},
		{name: "trailing space at boundary trimmed", s: helloWorld, maxLen: 6, want: "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
