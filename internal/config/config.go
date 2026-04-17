package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type ServerConfig struct {
	Listen           string   `json:"listen"`
	AdminBind        string   `json:"admin_bind"`
	AdminToken       string   `json:"admin_token"`
	EnrollmentTokens []string `json:"enrollment_tokens"`
}

type ClientConfig struct {
	ServerURL         string        `json:"server_url"`
	EnrollmentToken   string        `json:"enrollment_token"`
	NodeName          string        `json:"node_name"`
	PollInterval      time.Duration `json:"poll_interval"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

func LoadServerConfig(path string) (ServerConfig, error) {
	var cfg ServerConfig
	if err := loadJSON(path, &cfg); err != nil {
		return ServerConfig{}, err
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8443"
	}
	if cfg.AdminBind == "" {
		cfg.AdminBind = ":8080"
	}
	return cfg, nil
}

func LoadClientConfig(path string) (ClientConfig, error) {
	var cfg ClientConfig
	if err := loadJSON(path, &cfg); err != nil {
		return ClientConfig{}, err
	}
	if cfg.ServerURL == "" {
		return ClientConfig{}, fmt.Errorf("server_url is required")
	}
	if cfg.EnrollmentToken == "" {
		return ClientConfig{}, fmt.Errorf("enrollment_token is required")
	}
	if cfg.NodeName == "" {
		hostname, _ := os.Hostname()
		cfg.NodeName = hostname
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 15 * time.Second
	}
	return cfg, nil
}

func loadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
