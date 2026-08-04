//nolint:goconst // Test fixtures use repeated strings for readability.
package dto

import (
	"encoding/json"
	"reflect"
	"testing"

	"gud/internal/config"
)

func TestToEntity(t *testing.T) {
	tests := []struct {
		name string
		dto  ConfigDTO
		want config.Config
	}{
		{
			name: "all fields set",
			dto: ConfigDTO{
				DetailLevel: "detailed",
				Profile:     "computer-scientist",
				Model:       "gemini-flash-latest",
				Hint:        "focus on security",
				History:     config.Ptr(10),
				APIKey:      "sk-test",
				WrapLine:    100,
			},
			want: config.Config{
				DetailLevel: config.DetailDetailed,
				Profile:     config.ProfileName("computer-scientist"),
				Model:       "gemini-flash-latest",
				Hint:        "focus on security",
				History:     config.Ptr(10),
				APIKey:      "sk-test",
				WrapLine:    100,
			},
		},
		{
			name: "partial fields set",
			dto: ConfigDTO{
				Model:  "gemini-flash-lite-latest",
				APIKey: "sk-partial",
			},
			want: config.Config{
				Model:  "gemini-flash-lite-latest",
				APIKey: "sk-partial",
			},
		},
		{
			name: "empty dto",
			dto:  ConfigDTO{},
			want: config.Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dto.ToEntity()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToEntity() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFromEntity(t *testing.T) {
	tests := []struct {
		name   string
		entity config.Config
		want   ConfigDTO
	}{
		{
			name: "all fields set",
			entity: config.Config{
				DetailLevel: config.DetailDetailed,
				Profile:     config.ProfileName("astrophysicist"),
				Model:       "gemini-flash-lite-latest",
				Hint:        "explain physics",
				History:     config.Ptr(3),
				APIKey:      "sk-123",
				WrapLine:    72,
			},
			want: ConfigDTO{
				DetailLevel: "detailed",
				Profile:     "astrophysicist",
				Model:       "gemini-flash-lite-latest",
				Hint:        "explain physics",
				History:     config.Ptr(3),
				APIKey:      "sk-123",
				WrapLine:    72,
			},
		},
		{
			name:   "empty entity",
			entity: config.Config{},
			want:   ConfigDTO{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromEntity(tt.entity)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromEntity() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := config.Config{
		DetailLevel: config.DetailStandard,
		Profile:     config.ProfileName("chemist"),
		Model:       "gemini-flash-latest",
		Hint:        "mention reaction mechanisms",
		History:     config.Ptr(8),
		APIKey:      "sk-roundtrip",
		WrapLine:    80,
	}

	dto := FromEntity(original)
	entity := dto.ToEntity()

	if !reflect.DeepEqual(entity, original) {
		t.Errorf("Round-trip failed:\ngot  %+v\nwant %+v", entity, original)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := ConfigDTO{
		DetailLevel: "standard",
		Profile:     "biologist",
		Model:       "gemini-flash-lite-latest",
		History:     config.Ptr(5),
		WrapLine:    72,
	}

	//nolint:gosec // Test data, not real credentials.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored ConfigDTO
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(restored, original) {
		t.Errorf("JSON round-trip failed:\ngot  %+v\nwant %+v", restored, original)
	}

	// Verify omitempty — zero/empty fields should not appear
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if _, exists := raw["hint"]; exists {
		t.Errorf("hint should be omitted when empty, got %v", raw)
	}
	if _, exists := raw["api_key"]; exists {
		t.Errorf("api_key should be omitted when empty, got %v", raw)
	}
}

// TestJSONRoundTripHistoryZero is the DTO-level regression: an explicit
// "history": 0 must serialize (not be omitted by omitempty) and survive a JSON
// round-trip as a set pointer, so file config can disable history.
func TestJSONRoundTripHistoryZero(t *testing.T) {
	original := ConfigDTO{History: config.Ptr(0)}

	//nolint:gosec // Test data, not real credentials.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if v, exists := raw["history"]; !exists {
		t.Errorf("history should be serialized when explicitly 0, got %v", raw)
	} else if v != float64(0) {
		t.Errorf("history = %v, want 0", v)
	}

	var restored ConfigDTO
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if restored.History == nil || *restored.History != 0 {
		t.Errorf("round-tripped History = %v, want pointer to 0", restored.History)
	}
}

func TestJSONFieldNames(t *testing.T) {
	dto := ConfigDTO{
		DetailLevel: "minimal",
		APIKey:      "sk-test",
		WrapLine:    72,
	}

	//nolint:gosec // Test data, not real credentials.
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}

	expectedKeys := []string{"detail_level", "api_key", "wrapline"}
	for _, key := range expectedKeys {
		if _, exists := raw[key]; !exists {
			t.Errorf("JSON key %q not found in %v", key, raw)
		}
	}
}

func TestEmptyDTO(t *testing.T) {
	dto := ConfigDTO{}
	entity := dto.ToEntity()

	if entity.DetailLevel != "" {
		t.Errorf("empty DTO: DetailLevel = %q", entity.DetailLevel)
	}
	if entity.History != nil {
		t.Errorf("empty DTO: History = %v, want nil", entity.History)
	}
	if entity.WrapLine != 0 {
		t.Errorf("empty DTO: WrapLine = %d", entity.WrapLine)
	}
}
