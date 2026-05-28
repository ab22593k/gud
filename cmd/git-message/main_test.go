package main

import (
	"testing"

	"gud/internal/request"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name         string
		inputDetail  request.DetailLevel
		inputPersona request.PersonaName
		wantDetail   request.DetailLevel
		wantPersona  request.PersonaName
	}{
		{
			name:         "valid minimal detail level preserved",
			inputDetail:  request.DetailMinimal,
			inputPersona: request.PersonaEmbedded,
			wantDetail:   request.DetailMinimal,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "valid standard detail level preserved",
			inputDetail:  request.DetailStandard,
			inputPersona: request.PersonaEmbedded,
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "valid detailed detail level preserved",
			inputDetail:  request.DetailDetailed,
			inputPersona: request.PersonaEmbedded,
			wantDetail:   request.DetailDetailed,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "invalid detail level defaults to standard",
			inputDetail:  "verbose",
			inputPersona: request.PersonaEmbedded,
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "empty detail level defaults to standard",
			inputDetail:  "",
			inputPersona: request.PersonaEmbedded,
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "valid embedded persona preserved",
			inputDetail:  request.DetailStandard,
			inputPersona: request.PersonaEmbedded,
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "invalid persona defaults to embedded",
			inputDetail:  request.DetailStandard,
			inputPersona: "google",
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "empty persona defaults to embedded",
			inputDetail:  request.DetailStandard,
			inputPersona: "",
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "both invalid values default correctly",
			inputDetail:  "ultra",
			inputPersona: "unknown",
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "minimal detail with invalid persona",
			inputDetail:  request.DetailMinimal,
			inputPersona: "claude",
			wantDetail:   request.DetailMinimal,
			wantPersona:  request.PersonaEmbedded,
		},
		{
			name:         "detailed detail with empty persona",
			inputDetail:  request.DetailDetailed,
			inputPersona: "",
			wantDetail:   request.DetailDetailed,
			wantPersona:  request.PersonaEmbedded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				DetailLevel: tt.inputDetail,
				Persona:     tt.inputPersona,
			}

			c.validate()

			if c.DetailLevel != tt.wantDetail {
				t.Errorf("DetailLevel = %q, want %q", c.DetailLevel, tt.wantDetail)
			}
			if c.Persona != tt.wantPersona {
				t.Errorf("Persona = %q, want %q", c.Persona, tt.wantPersona)
			}
		})
	}
}
