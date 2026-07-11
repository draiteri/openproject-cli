// Package config loads configuration from environment variables and .env files.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all configuration for the OpenProject CLI.
type Config struct {
	Username string
	Password string
	BaseURL  string
}

// Load reads configuration from environment variables.
// Variables:
//   - OPENPROJECT_USERNAME (required)
//   - OPENPROJECT_PASSWORD (required)
//   - OPENPROJECT_URL      (optional, default: https://openproject.danieleraiteri.duckdns.org)
func Load() (*Config, error) {
	username := os.Getenv("OPENPROJECT_USERNAME")
	password := os.Getenv("OPENPROJECT_PASSWORD")
	baseURL := os.Getenv("OPENPROJECT_URL")

	if username == "" {
		return nil, fmt.Errorf("OPENPROJECT_USERNAME is not set")
	}
	if password == "" {
		return nil, fmt.Errorf("OPENPROJECT_PASSWORD is not set")
	}
	if baseURL == "" {
		baseURL = "https://openproject.danieleraiteri.duckdns.org"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &Config{
		Username: username,
		Password: password,
		BaseURL:  baseURL,
	}, nil
}

// LoadOrNil loads config but returns nil instead of error if env vars are missing.
// Useful for commands that don't require authentication (like --version).
func LoadOrNil() *Config {
	cfg, err := Load()
	if err != nil {
		return nil
	}
	return cfg
}
