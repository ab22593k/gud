package profile

// Package profile manages AI agent profile caching and remote fetching.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const catalogURL = "https://raw.githubusercontent.com/K-Dense-AI/scientific-agents/main/catalog.json"

const profileBaseURL = "https://raw.githubusercontent.com/K-Dense-AI/scientific-agents/" +
	"main/scientific-agents/%s/AGENTS.md"

type RemoteCatalog struct {
	Agents []RemoteAgent `json:"agents"`
}

type RemoteAgent struct {
	Profession string `json:"profession"`
	Summary    string `json:"summary"`
	Path       string `json:"path"`
	Slug       string `json:"slug,omitempty"`
	WorkMode   string `json:"work_mode"`
}

func (m *Manager) FetchCatalog(ctx context.Context) ([]CatalogEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog fetch returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var rc RemoteCatalog
	if err := json.Unmarshal(body, &rc); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	var entries []CatalogEntry
	for _, a := range rc.Agents {
		slug := slugify(a.Profession)
		entries = append(entries, CatalogEntry{
			Profession: a.Profession,
			Slug:       slug,
			Path:       a.Path,
			WorkMode:   a.WorkMode,
			Summary:    a.Summary,
		})
	}

	return entries, nil
}

func (m *Manager) FetchProfile(ctx context.Context, slug string) (string, error) {
	url := fmt.Sprintf(profileBaseURL, slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch profile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("profile %q not found on remote", slug)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("remote fetch returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return string(body), nil
}

func slugify(profession string) string {
	s := strings.ToLower(profession)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "&", "and")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "'", "")

	return s
}

func GetDownloadETA(estimatedCount int) time.Duration {
	return time.Duration(estimatedCount) * 500 * time.Millisecond
}
