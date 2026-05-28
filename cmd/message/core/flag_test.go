package cli

import (
	"bytes"
	"strings"
	"testing"

	"gud/internal/request"
)

func TestValidateConfig(t *testing.T) {
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
			name:         "valid conventional persona preserved",
			inputDetail:  request.DetailStandard,
			inputPersona: request.PersonaConventional,
			wantDetail:   request.DetailStandard,
			wantPersona:  request.PersonaConventional,
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
			// Save original config and restore after test
			origDetail := cfg.DetailLevel
			origPersona := cfg.Persona
			defer func() {
				cfg.DetailLevel = origDetail
				cfg.Persona = origPersona
			}()

			cfg.DetailLevel = tt.inputDetail
			cfg.Persona = tt.inputPersona

			validateConfig()

			if cfg.DetailLevel != tt.wantDetail {
				t.Errorf("DetailLevel = %q, want %q", cfg.DetailLevel, tt.wantDetail)
			}
			if cfg.Persona != tt.wantPersona {
				t.Errorf("Persona = %q, want %q", cfg.Persona, tt.wantPersona)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "gud version") {
		t.Errorf("version output = %q, want to contain 'gud version'", output)
	}
}

func TestRootCommandHelp(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("help output should contain 'Usage:', got %q", output)
	}
	if !strings.Contains(output, "hook") {
		t.Errorf("help output should list 'hook' subcommand, got %q", output)
	}
	if !strings.Contains(output, "version") {
		t.Errorf("help output should list 'version' subcommand, got %q", output)
	}
}

func TestHookCommandHelp(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"hook", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("hook help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "install") {
		t.Errorf("hook help output should contain 'install', got %q", output)
	}
	if !strings.Contains(output, "uninstall") {
		t.Errorf("hook help output should contain 'uninstall', got %q", output)
	}
	if !strings.Contains(output, "run") {
		t.Errorf("hook help output should contain 'run', got %q", output)
	}
}
