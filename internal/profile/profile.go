// Package profile manages AI agent profile caching and remote fetching.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Profile struct {
	Slug       string `json:"slug"`
	Profession string `json:"profession"`
	WorkMode   string `json:"work_mode,omitempty"`
	Content    string `json:"content,omitempty"`
}

type CatalogEntry struct {
	Profession string `json:"profession"`
	Slug       string `json:"slug"`
	Path       string `json:"path"`
	WorkMode   string `json:"work_mode"`
	Summary    string `json:"summary"`
}

type Manager struct {
	cacheDir string
}

func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	cacheDir := filepath.Join(home, ".config", "gud", "profiles")
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	return &Manager{cacheDir: cacheDir}, nil
}

func (m *Manager) List() ([]Profile, error) {
	var profiles []Profile

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return profiles, nil
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.cacheDir, entry.Name()))
		if err != nil {
			continue
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.Slug != "" {
			profiles = append(profiles, p)
		}
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Slug < profiles[j].Slug
	})

	return profiles, nil
}

func (m *Manager) cachePath(slug string) string {
	return filepath.Join(m.cacheDir, slug+".json")
}

func (m *Manager) IsCached(slug string) bool {
	_, err := os.Stat(m.cachePath(slug))

	return err == nil
}

func (m *Manager) Save(slug string, p Profile) error {
	p.Slug = slug
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(m.cachePath(slug), data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

func (m *Manager) Remove(slug string) error {
	if err := os.Remove(m.cachePath(slug)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q not found in cache", slug)
		}

		return fmt.Errorf("remove: %w", err)
	}

	return nil
}

func (m *Manager) Get(slug string) (*Profile, error) {
	data, err := os.ReadFile(m.cachePath(slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("profile %q not found (use 'gud profile save %s' first)", slug, slug)
		}

		return nil, fmt.Errorf("read: %w", err)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return &p, nil
}
