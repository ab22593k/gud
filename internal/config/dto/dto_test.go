//nolint:goconst // Test fixtures use repeated strings for readability.
package dto

import (
	"encoding/json"
	"testing"

	"gud/internal/config"
)

func TestToEntity(t *testing.T) {
	dto := ConfigDTO{
		DetailLevel: "detailed",
		Profile:     "computer-scientist",
		Model:       "gemini-3.1-pro",
		Temperature: 0.7,
		Hint:        "focus on security",
		History:     10,
		APIKey:      "sk-test",
		ACP:         "opencode",
		WrapLine:    100,
	}

	entity := dto.ToEntity()

	if string(entity.DetailLevel) != "detailed" {
		t.Errorf("DetailLevel = %q, want %q", entity.DetailLevel, "detailed")
	}
	if string(entity.Profile) != "computer-scientist" {
		t.Errorf("Profile = %q", entity.Profile)
	}
	if entity.Model != "gemini-3.1-pro" {
		t.Errorf("Model = %q", entity.Model)
	}
	if entity.Temperature != 0.7 {
		t.Errorf("Temperature = %v", entity.Temperature)
	}
	if entity.Hint != "focus on security" {
		t.Errorf("Hint = %q", entity.Hint)
	}
	if entity.History != 10 {
		t.Errorf("History = %d", entity.History)
	}
	if entity.APIKey != "sk-test" {
		t.Errorf("APIKey = %q", entity.APIKey)
	}
	if string(entity.ACP) != "opencode" {
		t.Errorf("ACP = %q", entity.ACP)
	}
	if entity.WrapLine != 100 {
		t.Errorf("WrapLine = %d", entity.WrapLine)
	}
}

func TestFromEntity(t *testing.T) {
	entity := config.Config{
		DetailLevel: config.DetailDetailed,
		Profile:     config.ProfileName("astrophysicist"),
		Model:       "gemini-3.1-flash-lite",
		Temperature: 1.0,
		Hint:        "explain physics",
		History:     3,
		APIKey:      "sk-123",
		ACP:         config.ACPGemini,
		WrapLine:    72,
	}

	dto := FromEntity(entity)

	if dto.DetailLevel != "detailed" {
		t.Errorf("DetailLevel = %q", dto.DetailLevel)
	}
	if dto.Profile != "astrophysicist" {
		t.Errorf("Profile = %q", dto.Profile)
	}
	if dto.Model != "gemini-3.1-flash-lite" {
		t.Errorf("Model = %q", dto.Model)
	}
	if dto.Temperature != 1.0 {
		t.Errorf("Temperature = %v", dto.Temperature)
	}
	if dto.Hint != "explain physics" {
		t.Errorf("Hint = %q", dto.Hint)
	}
	if dto.History != 3 {
		t.Errorf("History = %d", dto.History)
	}
	if dto.APIKey != "sk-123" {
		t.Errorf("APIKey = %q", dto.APIKey)
	}
	if dto.ACP != "gemini" {
		t.Errorf("ACP = %q", dto.ACP)
	}
	if dto.WrapLine != 72 {
		t.Errorf("WrapLine = %d", dto.WrapLine)
	}
}

func TestRoundTrip(t *testing.T) {
	original := config.Config{
		DetailLevel: config.DetailStandard,
		Profile:     config.ProfileName("chemist"),
		Model:       "gemini-3.1-pro",
		Temperature: 0.5,
		Hint:        "mention reaction mechanisms",
		History:     8,
		APIKey:      "sk-roundtrip",
		ACP:         config.ACPOpencode,
		WrapLine:    80,
	}

	dto := FromEntity(original)
	entity := dto.ToEntity()

	if entity != original {
		t.Errorf("Round-trip failed:\ngot  %+v\nwant %+v", entity, original)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := ConfigDTO{
		DetailLevel: "standard",
		Profile:     "biologist",
		Model:       "gemini-3.1-flash-lite",
		Temperature: 0.8,
		History:     5,
		ACP:         "gemini",
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

	if restored != original {
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

func TestJSONFieldNames(t *testing.T) {
	dto := ConfigDTO{
		DetailLevel: "minimal",
		ACP:         "gemini",
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

	expectedKeys := []string{"detail_level", "acp", "api_key", "wrapline"}
	for _, key := range expectedKeys {
		if _, exists := raw[key]; !exists {
			t.Errorf("JSON key %q not found in %v", key, raw)
		}
	}
}

func TestEmptyDTO(t *testing.T) {
	dto := ConfigDTO{}
	entity := dto.ToEntity()

	// All fields should be zero-valued
	if entity.DetailLevel != "" {
		t.Errorf("empty DTO: DetailLevel = %q", entity.DetailLevel)
	}
	if entity.ACP != "" {
		t.Errorf("empty DTO: ACP = %q", entity.ACP)
	}
	if entity.WrapLine != 0 {
		t.Errorf("empty DTO: WrapLine = %d", entity.WrapLine)
	}
}
