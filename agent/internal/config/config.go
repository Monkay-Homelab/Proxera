package config

import (
	"fmt"
	"os"

	"github.com/proxera/agent/pkg/types"
	"gopkg.in/yaml.v3"
)

// Load reads and parses the agent configuration file
func Load(path string) (*types.AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config types.AgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.AgentPort == 0 {
		config.AgentPort = 52080 // Dynamic port range to avoid conflicts
	}
	if config.NginxBinary == "" {
		config.NginxBinary = "/usr/sbin/nginx"
	}
	if config.NginxConfigPath == "" {
		config.NginxConfigPath = "/etc/nginx/conf.d"
	}
	if config.NginxEnabledPath == "" {
		config.NginxEnabledPath = "/etc/nginx/conf.d"
	}
	if config.MetricsInterval == 0 {
		config.MetricsInterval = 300 // 5 minutes default
	}

	return &config, nil
}

// Save writes the agent configuration to a file
func Save(config *types.AgentConfig, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
