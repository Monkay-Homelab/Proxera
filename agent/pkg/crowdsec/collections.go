package crowdsec

import (
	"fmt"
	"os/exec"
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
	if err := runJSON([]string{"collections", "list", "-o", "json"}, &collections); err != nil {
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
	cmd := exec.Command("cscli", "collections", "install", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install collection %s: %s", name, string(output))
	}
	return nil
}

// RemoveCollection removes a CrowdSec collection
func (m *Manager) RemoveCollection(name string) error {
	if !validCollectionName.MatchString(name) {
		return fmt.Errorf("invalid collection name: %s (expected format: author/name)", name)
	}
	cmd := exec.Command("cscli", "collections", "remove", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove collection %s: %s", name, string(output))
	}
	return nil
}
