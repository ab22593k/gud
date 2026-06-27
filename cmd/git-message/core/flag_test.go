package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/config/provider"
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

func TestConfigFromEnv(t *testing.T) {
	//nolint:gosec // Tests use fake credentials, never real secrets.
	const testAPIKey = "test-env-api-key"
	const testModel = "test-env-model"

	t.Setenv("GUD_DETAIL_LEVEL", "minimal")
	t.Setenv("GUD_PROFILE", "env-profile")
	t.Setenv("GUD_MODEL", testModel)
	t.Setenv("GUD_TEMPERATURE", "0.42")
	t.Setenv("GUD_HINT", "env-hint")
	t.Setenv("GUD_HISTORY", "7")
	t.Setenv("GUD_API_KEY", testAPIKey)
	t.Setenv("GUD_WRAPLINE", "100")

	cfg := configFromEnv()

	if cfg.DetailLevel != config.DetailMinimal {
		t.Errorf("DetailLevel = %q, want %q", cfg.DetailLevel, config.DetailMinimal)
	}
	if cfg.Profile != config.ProfileName("env-profile") {
		t.Errorf("Profile = %q", cfg.Profile)
	}
	if cfg.Model != testModel {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.Temperature != 0.42 {
		t.Errorf("Temperature = %v", cfg.Temperature)
	}
	if cfg.Hint != "env-hint" {
		t.Errorf("Hint = %q", cfg.Hint)
	}
	if cfg.History != 7 {
		t.Errorf("History = %d", cfg.History)
	}
	if cfg.APIKey != testAPIKey {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.WrapLine != 100 {
		t.Errorf("WrapLine = %d", cfg.WrapLine)
	}
}

func TestConfigFromEnvAliases(t *testing.T) {
	// OPENCODE_API_KEY should be picked up when GUD_API_KEY is unset
	t.Setenv("OPENCODE_API_KEY", "opencode-key")

	cfg := configFromEnv()

	if cfg.APIKey != "opencode-key" {
		t.Errorf("APIKey via OPENCODE_API_KEY = %q, want %q", cfg.APIKey, "opencode-key")
	}

	// GUD_API_KEY should take precedence over OPENCODE_API_KEY
	t.Setenv("GUD_API_KEY", "gud-prefers")

	cfg = configFromEnv()

	if cfg.APIKey != "gud-prefers" {
		t.Errorf("APIKey via GUD_API_KEY = %q, want %q", cfg.APIKey, "gud-prefers")
	}
}

func TestConfigFromEnvUnset(t *testing.T) {
	// No env vars set — all fields should be zero-valued
	cfg := configFromEnv()

	if cfg.DetailLevel != "" {
		t.Errorf("DetailLevel = %q, want empty", cfg.DetailLevel)
	}
	if cfg.Profile != "" {
		t.Errorf("Profile = %q, want empty", cfg.Profile)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty", cfg.Model)
	}
	if cfg.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0", cfg.Temperature)
	}
	if cfg.Hint != "" {
		t.Errorf("Hint = %q, want empty", cfg.Hint)
	}
	if cfg.History != 0 {
		t.Errorf("History = %d, want 0", cfg.History)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.WrapLine != 0 {
		t.Errorf("WrapLine = %d, want 0", cfg.WrapLine)
	}
}

func TestConfigPriorityChain(t *testing.T) {
	// Setup: XDG config file (fourth priority)
	xdgDir := t.TempDir()
	t.Setenv("HOME", xdgDir)
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

	// Setup: CWD config file (third priority)
	cwdDir := t.TempDir()
	cwdP := provider.NewFileProvider(filepath.Join(cwdDir, "gud.json"))
	if err := cwdP.Save(config.Config{
		Model:   "cwd-model",
		History: 10,
		APIKey:  "cwd-key",
	}); err != nil {
		t.Fatalf("save CWD config: %v", err)
	}

	// Setup: env config (second highest)
	t.Setenv("GUD_TEMPERATURE", "0.5")
	t.Setenv("GUD_MODEL", "env-model")
	t.Setenv("GUD_HISTORY", "3")

	// Setup: CLI config (highest priority)
	cliCfg := config.Config{
		Temperature: 0.99,
		WrapLine:    120,
	}

	// Manually layer the priority chain (can't easily change CWD in tests)
	result := config.DefaultConfig()
	result = result.Merge(loadConfigFile()) // XDG
	result = result.Merge(loadConfigFrom(
		filepath.Join(cwdDir, "gud.json"))) // CWD
	result = result.Merge(configFromEnv()) // env
	result = result.Merge(cliCfg)          // CLI
	result = result.Validate()

	// CLI overrides all
	if result.Temperature != 0.99 {
		t.Errorf("Temperature (CLI) = %v, want 0.99", result.Temperature)
	}
	if result.WrapLine != 120 {
		t.Errorf("WrapLine (CLI) = %d, want 120", result.WrapLine)
	}

	// Env overrides files but not CLI
	if result.Model != "env-model" {
		t.Errorf("Model (env) = %q, want env-model", result.Model)
	}
	if result.History != 3 {
		t.Errorf("History (env) = %d, want 3", result.History)
	}

	// CWD overrides XDG but not env/CLI
	if result.APIKey != "cwd-key" {
		t.Errorf("APIKey (CWD) = %q, want cwd-key", result.APIKey)
	}

	// XDG fills in when nothing else overrides
	if result.DetailLevel != config.DetailDetailed {
		t.Errorf("DetailLevel (XDG) = %q, want detailed", result.DetailLevel)
	}
}

func TestLoadCWDConfig(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cfgDir := t.TempDir()
	if err := os.Chdir(cfgDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	// No file yet — should return zero value
	cfg := loadCWDConfig()
	if cfg.Model != "" {
		t.Errorf("no CWD config: Model = %q, want empty", cfg.Model)
	}

	// Create gud.json in CWD
	p := provider.NewFileProvider(filepath.Join(cfgDir, "gud.json"))
	saved := config.Config{Model: "cwd-test", History: 7}
	if err := p.Save(saved); err != nil {
		t.Fatalf("save: %v", err)
	}

	cfg = loadCWDConfig()
	if cfg.Model != "cwd-test" {
		t.Errorf("CWD config: Model = %q, want cwd-test", cfg.Model)
	}
	if cfg.History != 7 {
		t.Errorf("CWD config: History = %d, want 7", cfg.History)
	}
}

func TestConfigPriorityOnlyDefaults(t *testing.T) {
	// With no file, no env, no CLI — should get DefaultConfig
	cfg := loadMergedConfig(config.Config{})

	defaults := config.DefaultConfig()
	validated := defaults.Validate()

	if cfg != validated {
		t.Errorf("only defaults: got %+v, want %+v", cfg, validated)
	}
}

func TestConfigPriorityOnlyCLI(t *testing.T) {
	cliCfg := config.Config{
		DetailLevel: config.DetailMinimal,
		Model:       "cli-model",
		History:     1,
		WrapLine:    50,
	}

	cfg := loadMergedConfig(cliCfg)

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
