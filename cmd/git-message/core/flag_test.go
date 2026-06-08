// Package core provides the CLI command structure and workflow orchestration for git-message.
package core

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
			cfg := Config{
				DetailLevel: tt.inputDetail,
				Persona:     tt.inputPersona,
			}

			got := validateConfig(cfg)

			if got.DetailLevel != tt.wantDetail {
				t.Errorf("DetailLevel = %q, want %q", got.DetailLevel, tt.wantDetail)
			}
			if got.Persona != tt.wantPersona {
				t.Errorf("Persona = %q, want %q", got.Persona, tt.wantPersona)
			}
		})
	}

	historyTests := []struct {
		name      string
		inputHist int
		wantHist  int
	}{
		{name: "history default (5) preserved", inputHist: 5, wantHist: 5},
		{name: "history 0 preserved", inputHist: 0, wantHist: 0},
		{name: "negative history clamped to 0", inputHist: -3, wantHist: 0},
		{name: "history above maxHistory clamped", inputHist: maxHistory + 10, wantHist: maxHistory},
		{name: "history at maxHistory preserved", inputHist: maxHistory, wantHist: maxHistory},
	}

	for _, tt := range historyTests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{History: tt.inputHist}
			got := validateConfig(cfg)
			if got.History != tt.wantHist {
				t.Errorf("History = %d, want %d", got.History, tt.wantHist)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetArgs(nil)
	})

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
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetArgs(nil)
	})

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
	if !strings.Contains(output, "--history") {
		t.Errorf("help output should include --history flag, got %q", output)
	}
}

func TestHookCommandHelp(t *testing.T) {
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetArgs(nil)
	})

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
