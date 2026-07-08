package mem

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	// DefaultContainerName is the default Docker container name for gud-managed HelixDB.
	DefaultContainerName = "gud-helixdb"
	// DefaultImage is the HelixDB Docker image.
	DefaultImage = "ghcr.io/helixdb/enterprise-dev"
	// DefaultInternalPort is the port HelixDB listens on inside the container.
	DefaultInternalPort = "8080"
	// readinessTimeout is how long to wait for the container to become healthy.
	readinessTimeout = 30 * time.Second
	// readinessPollInterval is how often to poll the health endpoint.
	readinessPollInterval = 500 * time.Millisecond
)

// ContainerManager manages a HelixDB Docker container lifecycle.
type ContainerManager struct {
	containerName string
	hostPort      string
	startedByUs   bool // true if we started the container this session
}

// NewContainerManager creates a manager for the given container name and host port.
func NewContainerManager(containerName, hostPort string) *ContainerManager {
	if containerName == "" {
		containerName = DefaultContainerName
	}
	if hostPort == "" {
		hostPort = "6969"
	}
	return &ContainerManager{
		containerName: containerName,
		hostPort:      hostPort,
	}
}

// ContainerName returns the Docker container name.
func (m *ContainerManager) ContainerName() string { return m.containerName }

// StartedByUs returns whether the container was started by this instance.
func (m *ContainerManager) StartedByUs() bool { return m.startedByUs }

// EnsureRunning checks if a HelixDB container is running and reachable.
// If no container exists, it pulls the image and starts one, then waits
// until the health endpoint responds. Returns the base URL and nil on success.
func (m *ContainerManager) EnsureRunning(ctx context.Context) (string, error) {
	// Check if container already exists and is running.
	switch m.containerStatus(ctx) {
	case "running":
		slog.Debug("helixdb: container already running",
			"container", m.containerName)
		return m.baseURL(), nil

	case "exited", "paused":
		slog.Debug("helixdb: restarting existing container",
			"container", m.containerName)
		if err := m.runDocker("start", m.containerName); err != nil {
			return "", fmt.Errorf("start container: %w", err)
		}
		m.startedByUs = true

	default:
		// Container doesn't exist — create and start it.
		slog.Debug("helixdb: starting new container",
			"container", m.containerName, "image", DefaultImage)
		if err := m.runDocker("run", "-d",
			"--name", m.containerName,
			"-p", m.hostPort+":"+DefaultInternalPort,
			DefaultImage,
		); err != nil {
			return "", fmt.Errorf("start container: %w", err)
		}
		m.startedByUs = true
	}

	// Wait for health endpoint.
	if err := m.waitForReadiness(ctx); err != nil {
		return "", fmt.Errorf("wait for helixdb readiness: %w", err)
	}

	slog.Debug("helixdb: container ready", "container", m.containerName,
		"url", m.baseURL())
	return m.baseURL(), nil
}

// Stop stops and removes the container. No-op if it wasn't started by us.
func (m *ContainerManager) Stop(ctx context.Context) error {
	if !m.startedByUs {
		return nil
	}
	slog.Debug("helixdb: stopping container", "container", m.containerName)
	_ = m.runDocker("rm", "-f", m.containerName)
	m.startedByUs = false
	return nil
}

// containerStatus returns the container state: "running", "exited", "paused", or "".
// Uses `docker ps -a` to detect containers in any state, not just running ones.
func (m *ContainerManager) containerStatus(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "name=^/"+m.containerName+"$",
		"--format", "{{.Status}}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	status := strings.TrimSpace(string(out))
	switch {
	case status == "":
		return ""
	case strings.HasPrefix(status, "Up "):
		return "running"
	case strings.HasPrefix(status, "Exited "):
		return "exited"
	case strings.HasPrefix(status, "Paused "):
		return "paused"
	default:
		return ""
	}
}

// waitForReadiness polls the health endpoint until it returns 200 or timeout.
func (m *ContainerManager) waitForReadiness(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()

	url := m.baseURL() + "/health"
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s: %w", url, ctx.Err())
		case <-time.After(readinessPollInterval):
		}
	}
}

// IsRunning returns true if the container is currently running.
func (m *ContainerManager) IsRunning(ctx context.Context) bool {
	return m.containerStatus(ctx) == "running"
}

// baseURL returns the HTTP URL for the managed container.
func (m *ContainerManager) baseURL() string {
	return "http://localhost:" + m.hostPort
}

// runDocker executes a docker command with the given args.
func (m *ContainerManager) runDocker(args ...string) error {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}
