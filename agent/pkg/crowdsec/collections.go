package crowdsec

import (
	"fmt"
	"regexp"
)

// validCollectionName matches CrowdSec collection names like "crowdsecurity/nginx" or "author/my-collection"
var validCollectionName = regexp.MustCompile(`^[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$`)

// Collection represents a CrowdSec collection
type Collection struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Version     string `json:"local_version"`
	Description string `json:"description"`
}

// ListCollections returns all installed collections
func (m *Manager) ListCollections() ([]Collection, error) {
	var collections []Collection
	if err := m.runJSON([]string{"collections", "list", "-o", "json"}, &collections); err != nil {
		return nil, err
	}
	if collections == nil {
		collections = []Collection{}
	}
	return collections, nil
}

// InstallCollection installs a CrowdSec collection
func (m *Manager) InstallCollection(name string) error {
	if !validCollectionName.MatchString(name) {
		return fmt.Errorf("invalid collection name: %s (expected format: author/name)", name)
	}
	_, err := m.runCscliCmd("collections", "install", name)
	if err != nil {
		return fmt.Errorf("failed to install collection %s: %v", name, err)
	}
	return nil
}

// RemoveCollection removes a CrowdSec collection
func (m *Manager) RemoveCollection(name string) error {
	if !validCollectionName.MatchString(name) {
		return fmt.Errorf("invalid collection name: %s (expected format: author/name)", name)
	}
	_, err := m.runCscliCmd("collections", "remove", name)
	if err != nil {
		return fmt.Errorf("failed to remove collection %s: %v", name, err)
	}
	return nil
}
