package mediator

import (
	"path/filepath"
	"testing"

	"gud/internal/config"
	"gud/internal/config/provider"
)

func TestConfigFromEnv(t *testing.T) {
	//nolint:gosec // Test uses fake credentials.
	const testAPIKey = "test-env-api-key"
	const testModel = "test-env-model"

	t.Setenv("GUD_DETAIL_LEVEL", "minimal")
	t.Setenv("GUD_PROFILE", "env-profile")
	t.Setenv("GUD_MODEL", testModel)
	t.Setenv("GUD_TEMPERATURE", "0.42")
	t.Setenv("GUD_HINT", "env-hint")
	t.Setenv("GUD_HISTORY", "7")
	t.Setenv("GOOGLE_API_KEY", testAPIKey)
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

func TestConfigFromEnvUnset(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
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

func TestMediatorLoad(t *testing.T) {
	xdgDir := t.TempDir()
	xdgPath := filepath.Join(xdgDir, "config.json")
	xdgP := provider.NewFileProvider(xdgPath)
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

	t.Setenv("GUD_TEMPERATURE", "0.5")
	t.Setenv("GUD_MODEL", "env-model")
	t.Setenv("GUD_HISTORY", "3")
	t.Setenv("GOOGLE_API_KEY", "")

	cliCfg := config.Config{
		Temperature: 0.99,
		WrapLine:    120,
	}

	m := &Mediator{XDGProvider: xdgP, CWDProvider: cwdP}
	cfg, err := m.Load(cliCfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// CLI overrides all
	if cfg.Temperature != 0.99 {
		t.Errorf("Temperature (CLI) = %v, want 0.99", cfg.Temperature)
	}
	if cfg.WrapLine != 120 {
		t.Errorf("WrapLine (CLI) = %d, want 120", cfg.WrapLine)
	}

	// Env overrides files but not CLI
	if cfg.Model != "env-model" {
		t.Errorf("Model (env) = %q, want env-model", cfg.Model)
	}
	if cfg.History != 3 {
		t.Errorf("History (env) = %d, want 3", cfg.History)
	}

	// CWD overrides XDG but not env/CLI
	if cfg.APIKey != "cwd-key" {
		t.Errorf("APIKey (CWD) = %q, want cwd-key", cfg.APIKey)
	}

	// XDG fills in when nothing else overrides
	if cfg.DetailLevel != config.DetailDetailed {
		t.Errorf("DetailLevel (XDG) = %q, want detailed", cfg.DetailLevel)
	}
}

func TestMediatorOnlyDefaults(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	xdgDir := t.TempDir()
	xdgP := provider.NewFileProvider(filepath.Join(xdgDir, "missing.json"))
	cwdP := provider.NewFileProvider(filepath.Join(xdgDir, "also-missing.json"))

	m := &Mediator{XDGProvider: xdgP, CWDProvider: cwdP}
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
	xdgDir := t.TempDir()
	xdgP := provider.NewFileProvider(filepath.Join(xdgDir, "missing.json"))
	cwdP := provider.NewFileProvider(filepath.Join(xdgDir, "also-missing.json"))

	cliCfg := config.Config{
		DetailLevel: config.DetailMinimal,
		Model:       "cli-model",
		History:     1,
		WrapLine:    50,
	}

	m := &Mediator{XDGProvider: xdgP, CWDProvider: cwdP}
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

func TestValidateStrictUnresolvedPlaceholder(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr string
	}{
		{
			name: "dollar brace placeholder in API key",
			//nolint:gosec // Test verifies placeholder detection; value is not a real credential.
			cfg:     config.Config{APIKey: "${GOOGLE_API_KEY}"},
			wantErr: "api_key contains unresolved placeholder",
		},
		{
			name:    "double brace placeholder in model",
			cfg:     config.Config{Model: "{{MODEL_NAME}}"},
			wantErr: "model contains unresolved placeholder",
		},
		{
			name:    "dollar paren placeholder in hint",
			cfg:     config.Config{Hint: "$(HINT)"},
			wantErr: "hint contains unresolved placeholder",
		},
		{
			name:    "no placeholders passes",
			cfg:     config.Config{APIKey: "real-key", Model: "gemini-2.0-flash"},
			wantErr: "",
		},
		{
			name:    "empty values pass",
			cfg:     config.Config{},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStrict(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateStrict = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("validateStrict = nil, want error containing %q", tt.wantErr)
				} else if !contains(err.Error(), tt.wantErr) {
					t.Errorf("validateStrict = %q, want error containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateStrictKnownPlaceholder(t *testing.T) {
	tests := []struct {
		apiKey string
		wantOK bool
	}{
		{"real-secret-key-12345", true},
		{"your-api-key", false},
		{"api-key", false},
		{"api_key", false},
		{"sk-your-key-here", false},
		{"", true},
		{"YOUR-API-KEY", false},
	}

	for _, tt := range tests {
		cfg := config.Config{APIKey: tt.apiKey}
		err := validateStrict(cfg)
		gotOK := err == nil
		if gotOK != tt.wantOK {
			t.Errorf("validateStrict(APIKey=%q): ok=%v, want %v (err=%v)", tt.apiKey, gotOK, tt.wantOK, err)
		}
	}
}

func TestHasPlaceholder(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"${VARIABLE}", true},
		{"{{VARIABLE}}", true},
		{"$(VARIABLE)", true},
		{"plain-text", false},
		{"mixed-${VAR}-text", true},
		{"", false},
	}

	for _, tt := range tests {
		got := hasPlaceholder(tt.input)
		if got != tt.want {
			t.Errorf("hasPlaceholder(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
