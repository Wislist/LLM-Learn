package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey      string `yaml:"api_key"`
	Model       string `yaml:"model"`
	MaxTurns    int    `yaml:"max_turns"`
	MaxHist     int    `yaml:"max_hist"`
	DBPath      string `yaml:"db_path"`
	AutoApprove bool   `yaml:"auto_approve"`
}

func LoadConfig() Config {
	cfg := Config{
		Model:    "deepseek-chat",
		MaxTurns: 10,
		MaxHist:  40,
		DBPath:   ".mini-opencode/sessions.db",
	}

	if data, err := os.ReadFile("config.yaml"); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	// 环境变量优先级更高，方便 CI / 容器场景覆盖。
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("DEEPSEEK_MODEL"); v != "" {
		cfg.Model = v
	}
	return cfg
}
