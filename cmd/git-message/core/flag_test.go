package core

import (
	"bytes"
	"strings"
	"testing"

	"gud/internal/config"
)

const (
	testUnknownProfile = "unknown"
	testHelpFlag       = "--help"
	testProfileCmdName = "profile"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name         string
		inputDetail  config.DetailLevel
		inputProfile config.ProfileName
		wantDetail   config.DetailLevel
		wantProfile  config.ProfileName
	}{
		{
			name:         "valid minimal detail level preserved",
			inputDetail:  config.DetailMinimal,
			inputProfile: "",
			wantDetail:   config.DetailMinimal,
			wantProfile:  "",
		},
		{
			name:         "valid standard detail level preserved",
			inputDetail:  config.DetailStandard,
			inputProfile: "",
			wantDetail:   config.DetailStandard,
			wantProfile:  "",
		},
		{
			name:         "valid detailed detail level preserved",
			inputDetail:  config.DetailDetailed,
			inputProfile: "",
			wantDetail:   config.DetailDetailed,
			wantProfile:  "",
		},
		{
			name:         "invalid detail level defaults to standard",
			inputDetail:  "verbose",
			inputProfile: "",
			wantDetail:   config.DetailStandard,
			wantProfile:  "",
		},
		{
			name:         "empty detail level defaults to standard",
			inputDetail:  "",
			inputProfile: "",
			wantDetail:   config.DetailStandard,
			wantProfile:  "",
		},
		{
			name:         "empty profile preserved as empty",
			inputDetail:  config.DetailStandard,
			inputProfile: "",
			wantDetail:   config.DetailStandard,
			wantProfile:  "",
		},
		{
			name:         "unknown profile preserved (cached remote profiles are valid)",
			inputDetail:  config.DetailStandard,
			inputProfile: "astrophysicist",
			wantDetail:   config.DetailStandard,
			wantProfile:  "astrophysicist",
		},
		{
			name:         "both detail invalid profile unknown",
			inputDetail:  "ultra",
			inputProfile: testUnknownProfile,
			wantDetail:   config.DetailStandard,
			wantProfile:  testUnknownProfile,
		},
		{
			name:         "minimal detail with cached profile",
			inputDetail:  config.DetailMinimal,
			inputProfile: "computer-scientist",
			wantDetail:   config.DetailMinimal,
			wantProfile:  "computer-scientist",
		},
		{
			name:         "detailed detail with empty profile",
			inputDetail:  config.DetailDetailed,
			inputProfile: "",
			wantDetail:   config.DetailDetailed,
			wantProfile:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				DetailLevel: tt.inputDetail,
				Profile:     tt.inputProfile,
			}

			got := cfg.Validate()

			if got.DetailLevel != tt.wantDetail {
				t.Errorf("DetailLevel = %q, want %q", got.DetailLevel, tt.wantDetail)
			}
			if got.Profile != tt.wantProfile {
				t.Errorf("Profile = %q, want %q", got.Profile, tt.wantProfile)
			}
		})
	}

	acpTests := []struct {
		name  string
		input config.ACPProvider
		want  config.ACPProvider
	}{
		{name: "valid opencode preserved", input: config.ACPOpencode, want: config.ACPOpencode},
		{name: "empty defaults to opencode", input: "", want: config.ACPOpencode},
		{name: "invalid value defaults to opencode", input: "unknown", want: config.ACPOpencode},
	}

	for _, tt := range acpTests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{ACP: tt.input}
			got := cfg.Validate()
			if got.ACP != tt.want {
				t.Errorf("ACP = %q, want %q", got.ACP, tt.want)
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
			cfg := config.Config{History: tt.inputHist}
			got := cfg.Validate()
			if got.History != tt.wantHist {
				t.Errorf("History = %d, want %d", got.History, tt.wantHist)
			}
		})
	}

	wrapLineTests := []struct {
		name   string
		input  int
		output int
	}{
		{name: "default 72 preserved", input: 72, output: 72},
		{name: "wrapline 100 preserved", input: 100, output: 100},
		{name: "below 40 clamped to 40", input: 20, output: 40},
		{name: "above 200 clamped to 200", input: 300, output: 200},
		{name: "exactly 40 preserved", input: 40, output: 40},
		{name: "exactly 200 preserved", input: 200, output: 200},
	}

	for _, tt := range wrapLineTests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{WrapLine: tt.input}
			got := cfg.Validate()
			if got.WrapLine != tt.output {
				t.Errorf("WrapLine = %d, want %d", got.WrapLine, tt.output)
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
	rootCmd.SetArgs([]string{testHelpFlag})

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
	if !strings.Contains(output, "profile") {
		t.Errorf("help output should list 'profile' subcommand, got %q", output)
	}
	if !strings.Contains(output, "version") {
		t.Errorf("help output should list 'version' subcommand, got %q", output)
	}
	if !strings.Contains(output, "--history") {
		t.Errorf("help output should include --history flag, got %q", output)
	}
	if !strings.Contains(output, "--wrapline") {
		t.Errorf("help output should include --wrapline flag, got %q", output)
	}
}

func TestProfileCommandHelp(t *testing.T) {
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetArgs(nil)
	})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{testProfileCmdName, testHelpFlag})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("profile help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "list") {
		t.Errorf("profile help should contain 'list', got %q", output)
	}
	if !strings.Contains(output, "save") {
		t.Errorf("profile help should contain 'save', got %q", output)
	}
	if !strings.Contains(output, "remove") {
		t.Errorf("profile help should contain 'remove', got %q", output)
	}
	if !strings.Contains(output, "show") {
		t.Errorf("profile help should contain 'show', got %q", output)
	}
}

func TestProfileListCommand(t *testing.T) {
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetArgs(nil)
	})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"profile", "list"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("profile list command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Cached profiles") && !strings.Contains(output, "No cached profiles") {
		t.Errorf("profile list output unexpected, got %q", output)
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
