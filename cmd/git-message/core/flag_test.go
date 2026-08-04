package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/config/provider"

	"github.com/spf13/cobra"
)

const (
	testUnknownProfile = "unknown"
	testHelpFlag       = "--help"
	testProfileCmdName = "profile"
	testVersionCmdName = "version"
	testListCmdName    = "list"
	testHookCmdName    = "hook"
	testAstrophysicist = "astrophysicist"
	testModelName      = "gemini-flash-latest"
	testDoneStr        = "done"
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
			inputProfile: testAstrophysicist,
			wantDetail:   config.DetailStandard,
			wantProfile:  testAstrophysicist,
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

func TestMediatorPriorityChain(t *testing.T) {
	xdgDir := t.TempDir()
	xdgCfgPath := filepath.Join(xdgDir, ".config", "gud", "config.json")
	if err := os.MkdirAll(filepath.Dir(xdgCfgPath), 0750); err != nil {
		t.Fatalf("mkdir XDG: %v", err)
	}
	xdgP := provider.NewFileProvider(xdgCfgPath)
	if err := xdgP.Save(config.Config{
		DetailLevel: config.DetailDetailed,
		Model:       "xdg-model",
		History:     20,
	}); err != nil {
		t.Fatalf("save XDG config: %v", err)
	}

	cwdDir := t.TempDir()
	cwdP := provider.NewFileProvider(filepath.Join(cwdDir, "gud.json"))
	if err := cwdP.Save(config.Config{
		Model:   "cwd-model",
		History: 10,
		APIKey:  "cwd-key",
	}); err != nil {
		t.Fatalf("save CWD config: %v", err)
	}

	t.Setenv("GUD_MODEL", "env-model")
	t.Setenv("GUD_HISTORY", "3")
	t.Setenv("GOOGLE_API_KEY", "")

	cliCfg := config.Config{
		WrapLine: 120,
	}

	m := &mediator.Mediator{XDGProvider: xdgP, CWDProvider: cwdP}
	cfg, err := m.Load(cliCfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.WrapLine != 120 {
		t.Errorf("WrapLine (CLI) = %d, want 120", cfg.WrapLine)
	}

	if cfg.Model != "env-model" {
		t.Errorf("Model (env) = %q, want env-model", cfg.Model)
	}
	if cfg.History != 3 {
		t.Errorf("History (env) = %d, want 3", cfg.History)
	}

	if cfg.APIKey != "cwd-key" {
		t.Errorf("APIKey (CWD) = %q, want cwd-key", cfg.APIKey)
	}

	if cfg.DetailLevel != config.DetailDetailed {
		t.Errorf("DetailLevel (XDG) = %q, want detailed", cfg.DetailLevel)
	}
}

func TestMediatorOnlyDefaults(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	td := t.TempDir()
	m := &mediator.Mediator{
		XDGProvider: provider.NewFileProvider(filepath.Join(td, "missing.json")),
		CWDProvider: provider.NewFileProvider(filepath.Join(td, "also-missing.json")),
	}
	cfg, err := m.Load(config.Config{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	defaults := config.DefaultConfig()
	validated := defaults.Validate()

	if cfg != validated {
		t.Errorf("only defaults: got %+v, want %+v", cfg, validated)
	}
}

func TestMediatorOnlyCLI(t *testing.T) {
	td := t.TempDir()
	m := &mediator.Mediator{
		XDGProvider: provider.NewFileProvider(filepath.Join(td, "missing.json")),
		CWDProvider: provider.NewFileProvider(filepath.Join(td, "also-missing.json")),
	}
	cliCfg := config.Config{
		DetailLevel: config.DetailMinimal,
		Model:       "cli-model",
		History:     1,
		WrapLine:    50,
	}

	cfg, err := m.Load(cliCfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DetailLevel != config.DetailMinimal {
		t.Errorf("DetailLevel = %q, want minimal", cfg.DetailLevel)
	}
	if cfg.Model != "cli-model" {
		t.Errorf("Model = %q, want cli-model", cfg.Model)
	}
	if cfg.History != 1 {
		t.Errorf("History = %d, want 1", cfg.History)
	}
	if cfg.WrapLine != 50 {
		t.Errorf("WrapLine = %d, want 50", cfg.WrapLine)
	}
}

// flagCommand builds a fresh command registered with the same persistent
// flags as production (via addPersistentFlags) and parses args, returning a
// command whose FlagSet reflects a realistic CLI invocation. This avoids
// mutating the package-level rootCmd shared by other tests.
func flagCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	addPersistentFlags(cmd)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags(%q): %v", args, err)
	}
	return cmd
}

func TestConfigFromCmdUnchangedFlags(t *testing.T) {
	// No flags on the command line: cobra defaults (standard/72/5) must NOT leak.
	cfg := configFromCmd(flagCommand(t))

	want := config.Config{}
	if cfg != want {
		t.Errorf("configFromCmd(default flags) = %+v, want zero-value %+v (no CLI overrides)", cfg, want)
	}
}

func TestConfigFromCmdChangedFlags(t *testing.T) {
	cfg := configFromCmd(flagCommand(t,
		"--detail-level", "detailed",
		"--history", "8",
		"--wrapline", "90",
	))

	if cfg.DetailLevel != config.DetailDetailed {
		t.Errorf("DetailLevel = %q, want detailed", cfg.DetailLevel)
	}
	if cfg.History != 8 {
		t.Errorf("History = %d, want 8", cfg.History)
	}
	if cfg.WrapLine != 90 {
		t.Errorf("WrapLine = %d, want 90", cfg.WrapLine)
	}
	// Unchanged flags stay zero so they don't override lower priorities.
	if cfg.Profile != "" || cfg.Model != "" || cfg.Hint != "" {
		t.Errorf("unchanged string flags set: %+v (want all empty)", cfg)
	}
}

// TestMediatorPreservesGudJSONWhenFlagsUnchanged is the integration-gap test:
// the prior TestMediatorLoad passed a zero-value/hand-built cliCfg which never
// modelled the real CLI layer (where cobra flag defaults leak in). Here cliCfg
// comes from configFromCmd on a real parsed command, so we prove that a user's
// gud.json {detail_level: detailed, wrapline: 100, history: 20} survives the
// full file → env → CLI pipeline when the user passes no flags.
func TestMediatorPreservesGudJSONWhenFlagsUnchanged(t *testing.T) {
	xdgP := provider.NewFileProvider(filepath.Join(t.TempDir(), "missing.json"))
	cwdP := provider.NewFileProvider(filepath.Join(t.TempDir(), "gud.json"))
	if err := cwdP.Save(config.Config{
		DetailLevel: config.DetailDetailed,
		WrapLine:    100,
		History:     20,
	}); err != nil {
		t.Fatalf("save gud.json: %v", err)
	}

	// Realistic CLI layer: same flag registration + parser, but user passes
	// no flags. The flag defaults must not clobber gud.json.
	cliCfg := configFromCmd(flagCommand(t))

	m := &mediator.Mediator{XDGProvider: xdgP, CWDProvider: cwdP}
	cfg, err := m.Load(cliCfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DetailLevel != config.DetailDetailed {
		t.Errorf("DetailLevel = %q, want detailed (gud.json preserved, not flag default)", cfg.DetailLevel)
	}
	if cfg.WrapLine != 100 {
		t.Errorf("WrapLine = %d, want 100 (gud.json preserved, not flag default 72)", cfg.WrapLine)
	}
	if cfg.History != 20 {
		t.Errorf("History = %d, want 20 (gud.json preserved, not flag default 5)", cfg.History)
	}
}

// TestMediatorCliOverridesGudJSON shows the inverse: an explicit flag still
// wins over gud.json, per documented priority "CLI flags → env → gud.json".
func TestMediatorCliOverridesGudJSON(t *testing.T) {
	xdgP := provider.NewFileProvider(filepath.Join(t.TempDir(), "missing.json"))
	cwdP := provider.NewFileProvider(filepath.Join(t.TempDir(), "gud.json"))
	if err := cwdP.Save(config.Config{
		DetailLevel: config.DetailDetailed,
		WrapLine:    100,
		History:     20,
	}); err != nil {
		t.Fatalf("save gud.json: %v", err)
	}

	cliCfg := configFromCmd(flagCommand(t,
		"--detail-level", "minimal",
		"--wrapline", "120",
		"--history", "2",
	))

	m := &mediator.Mediator{XDGProvider: xdgP, CWDProvider: cwdP}
	cfg, err := m.Load(cliCfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DetailLevel != config.DetailMinimal {
		t.Errorf("DetailLevel = %q, want minimal (explicit CLI wins)", cfg.DetailLevel)
	}
	if cfg.WrapLine != 120 {
		t.Errorf("WrapLine = %d, want 120 (explicit CLI wins)", cfg.WrapLine)
	}
	if cfg.History != 2 {
		t.Errorf("History = %d, want 2 (explicit CLI wins)", cfg.History)
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
	rootCmd.SetArgs([]string{testVersionCmdName})

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
	origIn := rootCmd.InOrStdin()
	t.Cleanup(func() {
		rootCmd.SetOut(origOut)
		rootCmd.SetIn(origIn)
		rootCmd.SetArgs(nil)
	})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetIn(&bytes.Buffer{}) // non-terminal stdin to prevent TUI launch
	rootCmd.SetArgs([]string{testProfileCmdName, testListCmdName})

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
	rootCmd.SetArgs([]string{testHookCmdName, testHelpFlag})

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
