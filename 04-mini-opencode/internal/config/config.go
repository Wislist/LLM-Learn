package config

import (
	"encoding/json"
	"errors"
	"os"
)

type Config struct {
	Provider   ProviderConfig             `json:"provider"`
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

type ProviderConfig struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key,omitempty"`
	APIKeyEnv string `json:"api_key_env"`
}

type MCPServerConfig struct {
	Enabled bool     `json:"enabled"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func Default() Config {
	return Config{
		Provider:   ProviderConfig{Name: "echo"},
		MCPServers: map[string]MCPServerConfig{},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Provider.Name == "" {
		cfg.Provider.Name = "echo"
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = map[string]MCPServerConfig{}
	}
	return cfg, nil
}

func (c ProviderConfig) ResolvedAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if c.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.APIKeyEnv)
}
