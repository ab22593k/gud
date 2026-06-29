package git

import (
	"testing"
)

func TestExtractCodeUnits_GoFunction(t *testing.T) {
	diff := `diff --git a/internal/request/client.go b/internal/request/client.go
index abc123..def456 100644
--- a/internal/request/client.go
+++ b/internal/request/client.go
@@ -10,7 +10,8 @@ func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
 	if cfg.APIKey == "" {
 		return nil, ErrMissingAPIKey
 	}
-	return &Client{apiKey: cfg.APIKey}, nil
+	client := &Client{apiKey: cfg.APIKey}
+	return client, nil
 }
`

	units := ExtractCodeUnits(diff)
	if len(units) == 0 {
		t.Fatal("expected at least one code unit")
	}
	if units[0].Name != "NewClient" {
		t.Errorf("expected name 'NewClient', got %q", units[0].Name)
	}
	if units[0].Kind != "function" {
		t.Errorf("expected kind 'function', got %q", units[0].Kind)
	}
	if units[0].FilePath != "internal/request/client.go" {
		t.Errorf("expected file path 'internal/request/client.go', got %q", units[0].FilePath)
	}
	if units[0].ChangeType != "modified" {
		t.Errorf("expected change type 'modified', got %q", units[0].ChangeType)
	}
}

func TestExtractCodeUnits_Method(t *testing.T) {
	diff := `diff --git a/internal/git/diff.go b/internal/git/diff.go
@@ -30,7 +30,7 @@ func (g *GitRepo) Commit(ctx context.Context, message string) error {
 	if err := cmd.Run(); err != nil {
 		return fmt.Errorf("git commit failed: %w", err)
 	}
-	return nil
+	return cmd.Wait()
 }
`

	units := ExtractCodeUnits(diff)
	if len(units) == 0 {
		t.Fatal("expected at least one code unit")
	}
	if units[0].Name != "(*GitRepo).Commit" {
		t.Errorf("expected name '(*GitRepo).Commit', got %q", units[0].Name)
	}
	if units[0].Kind != "method" {
		t.Errorf("expected kind 'method', got %q", units[0].Kind)
	}
}

func TestExtractCodeUnits_Struct(t *testing.T) {
	diff := `diff --git a/internal/config/config.go b/internal/config/config.go
@@ -1,5 +1,6 @@
-type Config struct {
+type Config struct {
 	DetailLevel DetailLevel
+	Verbose     bool
 }
`

	units := ExtractCodeUnits(diff)
	if len(units) == 0 {
		t.Fatal("expected at least one code unit")
	}
	if units[0].Name != "Config" {
		t.Errorf("expected name 'Config', got %q", units[0].Name)
	}
	if units[0].Kind != "struct" {
		t.Errorf("expected kind 'struct', got %q", units[0].Kind)
	}
}

func TestExtractCodeUnits_MultipleHunks(t *testing.T) {
	diff := `diff --git a/cmd/root.go b/cmd/root.go
@@ -10,7 +10,8 @@ func init() {
 	rootCmd.Flags().String("hint", "", "focus hint")
+	rootCmd.Flags().Int("history", 5, "history count")
 }
@@ -40,7 +40,7 @@ func Execute() error {
 	if err := rootCmd.Execute(); err != nil {
 		return err
 	}
-	return nil
+	return err
 }
`

	units := ExtractCodeUnits(diff)
	if len(units) != 2 {
		t.Fatalf("expected 2 code units, got %d", len(units))
	}
	if units[0].Name != "init" && units[1].Name != "init" {
		t.Errorf("expected at least one 'init' function")
	}
}
