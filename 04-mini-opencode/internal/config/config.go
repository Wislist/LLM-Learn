package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

type SecretStore struct {
	ProviderKeys map[string]string `json:"provider_keys"`
}

func Default() Config {
	return Config{
		Provider:   ProviderConfig{Name: "echo"},
		MCPServers: map[string]MCPServerConfig{},
	}
}

func DefaultDeepSeekProvider() ProviderConfig {
	return ProviderConfig{
		Name:      "deepseek",
		BaseURL:   "https://api.deepseek.com",
		Model:     "deepseek-chat",
		APIKeyEnv: "DEEPSEEK_API_KEY",
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

func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
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

func (c ProviderConfig) ResolvedAPIKeyFrom(workDir string) string {
	if key := c.ResolvedAPIKey(); key != "" {
		return key
	}
	store, err := LoadSecretStore(workDir)
	if err != nil {
		return ""
	}
	return store.ProviderKeys[c.Name]
}

func LoadSecretStore(workDir string) (SecretStore, error) {
	store := SecretStore{ProviderKeys: map[string]string{}}
	data, err := os.ReadFile(secretStorePath(workDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return store, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	if store.ProviderKeys == nil {
		store.ProviderKeys = map[string]string{}
	}
	return store, nil
}

func SaveProviderKey(workDir string, providerName string, key string) error {
	store, err := LoadSecretStore(workDir)
	if err != nil {
		return err
	}
	if store.ProviderKeys == nil {
		store.ProviderKeys = map[string]string{}
	}
	store.ProviderKeys[providerName] = key

	path := secretStorePath(workDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func secretStorePath(workDir string) string {
	return filepath.Join(workDir, ".mini-opencode", "secrets.json")
}
