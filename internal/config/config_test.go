//nolint:goconst // Test fixtures use repeated strings for readability.
package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DetailLevel != DetailStandard {
		t.Errorf("DefaultConfig().DetailLevel = %q, want %q", cfg.DetailLevel, DetailStandard)
	}
	if cfg.WrapLine != 72 {
		t.Errorf("DefaultConfig().WrapLine = %d, want 72", cfg.WrapLine)
	}
}

func TestValidateDetailLevel(t *testing.T) {
	tests := []struct {
		name  string
		input DetailLevel
		want  DetailLevel
	}{
		{name: "minimal preserved", input: DetailMinimal, want: DetailMinimal},
		{name: "standard preserved", input: DetailStandard, want: DetailStandard},
		{name: "detailed preserved", input: DetailDetailed, want: DetailDetailed},
		{name: "empty defaults to standard", input: "", want: DetailStandard},
		{name: "invalid defaults to standard", input: "verbose", want: DetailStandard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{DetailLevel: tt.input}
			got := cfg.Validate()
			if got.DetailLevel != tt.want {
				t.Errorf("Validate().DetailLevel = %q, want %q", got.DetailLevel, tt.want)
			}
		})
	}
}

func TestValidateHistory(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "positive preserved", input: 5, want: 5},
		{name: "zero preserved", input: 0, want: 0},
		{name: "negative clamped to 0", input: -1, want: 0},
		{name: "above max clamped", input: 100, want: maxHistory},
		{name: "at max preserved", input: maxHistory, want: maxHistory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{History: tt.input}
			got := cfg.Validate()
			if got.History != tt.want {
				t.Errorf("Validate().History = %d, want %d", got.History, tt.want)
			}
		})
	}
}

func TestValidateWrapLine(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "72 preserved", input: 72, want: 72},
		{name: "100 preserved", input: 100, want: 100},
		{name: "below min clamped", input: 20, want: minWrap},
		{name: "above max clamped", input: 300, want: maxWrap},
		{name: "at min preserved", input: minWrap, want: minWrap},
		{name: "at max preserved", input: maxWrap, want: maxWrap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{WrapLine: tt.input}
			got := cfg.Validate()
			if got.WrapLine != tt.want {
				t.Errorf("Validate().WrapLine = %d, want %d", got.WrapLine, tt.want)
			}
		})
	}
}

func TestMerge(t *testing.T) {
	base := Config{
		DetailLevel: DetailStandard,
		Model:       "gemini-3.1-flash-lite",
		Temperature: 1.0,
		History:     5,
		WrapLine:    72,
	}

	override := Config{
		Model:       "gemini-3.1-pro",
		Temperature: 0.5,
	}

	merged := base.Merge(override)

	if merged.DetailLevel != DetailStandard {
		t.Errorf("Merge preserved DetailLevel = %q, want %q", merged.DetailLevel, DetailStandard)
	}
	if merged.Model != "gemini-3.1-pro" {
		t.Errorf("Merge overrode Model = %q, want %q", merged.Model, "gemini-3.1-pro")
	}
	if merged.Temperature != 0.5 {
		t.Errorf("Merge overrode Temperature = %v, want %v", merged.Temperature, 0.5)
	}
	if merged.History != 5 {
		t.Errorf("Merge preserved History = %d, want 5", merged.History)
	}
	if merged.WrapLine != 72 {
		t.Errorf("Merge preserved WrapLine = %d, want 72", merged.WrapLine)
	}
}

func TestMergeEmptyOverride(t *testing.T) {
	base := Config{
		DetailLevel: DetailDetailed,
		Model:       "gemini-3.1-flash-lite",
	}

	merged := base.Merge(Config{})

	if merged.DetailLevel != DetailDetailed {
		t.Errorf("Merge empty preserved DetailLevel = %q", merged.DetailLevel)
	}
	if merged.Model != "gemini-3.1-flash-lite" {
		t.Errorf("Merge empty preserved Model = %q", merged.Model)
	}
}

func TestMergeOverrideEmptyBase(t *testing.T) {
	override := Config{
		Model:       "claude-4-opus",
		Temperature: 0.3,
		WrapLine:    80,
	}

	merged := Config{}.Merge(override)

	if merged.Model != "claude-4-opus" {
		t.Errorf("Merge from zero value: Model = %q", merged.Model)
	}
	if merged.Temperature != 0.3 {
		t.Errorf("Merge from zero value: Temperature = %v", merged.Temperature)
	}
	if merged.WrapLine != 80 {
		t.Errorf("Merge from zero value: WrapLine = %d", merged.WrapLine)
	}
}
