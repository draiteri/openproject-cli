// Package config loads configuration from environment variables and .env files.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all configuration for the OpenProject CLI.
type Config struct {
	APIKey   string
	Username string
	Password string
	BaseURL  string
}

// UsesAPIKey reports whether the config authenticates with an API key rather
// than a username/password session login.
func (c *Config) UsesAPIKey() bool {
	return c.APIKey != ""
}

// Load reads configuration from environment variables.
// Variables:
//   - OPENPROJECT_API_KEY  (preferred)
//   - OPENPROJECT_USERNAME (fallback, required if no API key)
//   - OPENPROJECT_PASSWORD (fallback, required if no API key)
//   - OPENPROJECT_URL      (optional, default: https://openproject.danieleraiteri.duckdns.org)
//
// If OPENPROJECT_API_KEY is set it takes precedence and the username/password
// are ignored. Otherwise the CLI falls back to the legacy session login.
func Load() (*Config, error) {
	apiKey := os.Getenv("OPENPROJECT_API_KEY")
	username := os.Getenv("OPENPROJECT_USERNAME")
	password := os.Getenv("OPENPROJECT_PASSWORD")
	baseURL := os.Getenv("OPENPROJECT_URL")

	if apiKey == "" {
		if username == "" || password == "" {
			return nil, fmt.Errorf("OPENPROJECT_API_KEY is not set (and no OPENPROJECT_USERNAME/OPENPROJECT_PASSWORD fallback available)")
		}
	}

	if baseURL == "" {
		baseURL = "https://openproject.danieleraiteri.duckdns.org"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &Config{
		APIKey:   apiKey,
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
