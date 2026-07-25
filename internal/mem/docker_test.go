package mem

import (
	"context"
	"testing"
)

func TestNewContainerManager_DefaultValues(t *testing.T) {
	t.Parallel()

	m := NewContainerManager("", "")
	if m.ContainerName() != DefaultContainerName {
		t.Errorf("ContainerName() = %q, want %q", m.ContainerName(), DefaultContainerName)
	}
	if m.StartedByUs() {
		t.Error("StartedByUs() = true, want false")
	}
}

func TestNewContainerManager_CustomValues(t *testing.T) {
	t.Parallel()

	m := NewContainerManager("my-helix", "8080")
	if m.ContainerName() != "my-helix" {
		t.Errorf("ContainerName() = %q, want my-helix", m.ContainerName())
	}
	if m.StartedByUs() {
		t.Error("StartedByUs() = true, want false")
	}
}

func TestContainerManager_StopNotStartedByUs(t *testing.T) {
	t.Parallel()

	m := NewContainerManager("test", "9999")
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop() should be no-op when not started by us, got: %v", err)
	}
	if m.StartedByUs() {
		t.Error("StartedByUs() should still be false after no-op Stop()")
	}
}

func TestContainerManager_IsRunningWithoutDocker(t *testing.T) {
	t.Parallel()

	m := NewContainerManager("test-container", "9999")
	if m.IsRunning(context.Background()) {
		t.Error("IsRunning() = true, want false (no Docker or container available)")
	}
}

func TestContainerManager_StartedByUsInitiallyFalse(t *testing.T) {
	t.Parallel()

	m := NewContainerManager("x", "0")
	if m.StartedByUs() {
		t.Error("new ContainerManager should have StartedByUs = false")
	}
}

func TestContainerManager_BaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		port string
		want string
	}{
		{name: "default port 6969", port: "6969", want: "http://localhost:6969"},
		{name: "custom port 8080", port: "8080", want: "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewContainerManager("x", tt.port)
			if got := m.baseURL(); got != tt.want {
				t.Errorf("baseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
