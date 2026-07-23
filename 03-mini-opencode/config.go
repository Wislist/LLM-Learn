package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type MCPServerConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type Config struct {
	APIKey              string                     `yaml:"api_key"`
	Model               string                     `yaml:"model"`
	MaxTurns            int                        `yaml:"max_turns"`
	MaxHist             int                        `yaml:"max_hist"`
	ContextWindow       int                        `yaml:"context_window"`
	OutputReserve       int                        `yaml:"output_reserve"`
	MemoryReserve       int                        `yaml:"memory_reserve"`
	CompressionRatio    float64                    `yaml:"compression_ratio"`
	DBPath              string                     `yaml:"db_path"`
	AutoApprove         bool                       `yaml:"auto_approve"`
	SkillsDir           string                     `yaml:"skills_dir"`
	MCP                 map[string]MCPServerConfig `yaml:"mcp"`
}

func LoadConfig() Config {
	cfg := Config{
		Model:            "deepseek-chat",
		MaxTurns:         10,
		MaxHist:          200,
		ContextWindow:    64 * 1024,
		OutputReserve:    8 * 1024,
		MemoryReserve:    4 * 1024,
		CompressionRatio: 0.8,
		DBPath:           ".mini-opencode/sessions.db",
		SkillsDir:        ".mini-opencode/skills",
	}

	if data, err := os.ReadFile("config.yaml"); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("DEEPSEEK_MODEL"); v != "" {
		cfg.Model = v
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = 64 * 1024
	}
	if cfg.OutputReserve <= 0 {
		cfg.OutputReserve = 8 * 1024
	}
	if cfg.MemoryReserve <= 0 {
		cfg.MemoryReserve = 4 * 1024
	}
	if cfg.CompressionRatio <= 0 || cfg.CompressionRatio >= 1 {
		cfg.CompressionRatio = 0.8
	}
	return cfg
}
