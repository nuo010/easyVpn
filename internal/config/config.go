package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"easyvpn/internal/control"
)

type ServerConfig struct {
	Listen         string         `json:"listen"`
	AdminBind      string         `json:"admin_bind"`
	AdminToken     string         `json:"admin_token"`
	EnableTunnel   bool           `json:"enable_tunnel"`
	PublicDataAddr string         `json:"public_data_addr"`
	TunnelName     string         `json:"tunnel_name"`
	TunnelAddress  string         `json:"tunnel_address"`
	TunnelMTU      int            `json:"tunnel_mtu"`
	Users          []control.User `json:"users"`
}

type ClientConfig struct {
	ServerURL         string        `json:"server_url"`
	Username          string        `json:"username"`
	Password          string        `json:"password"`
	NodeName          string        `json:"node_name"`
	EnableTunnel      bool          `json:"enable_tunnel"`
	TunnelName        string        `json:"tunnel_name"`
	ApplySystemRoutes bool          `json:"apply_system_routes"`
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
	if cfg.TunnelName == "" {
		cfg.TunnelName = "easyvpn0"
	}
	if cfg.TunnelAddress == "" {
		cfg.TunnelAddress = "10.200.0.1/24"
	}
	if cfg.TunnelMTU <= 0 {
		cfg.TunnelMTU = 1380
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
	if cfg.Username == "" {
		return ClientConfig{}, fmt.Errorf("username is required")
	}
	if cfg.Password == "" {
		return ClientConfig{}, fmt.Errorf("password is required")
	}
	if cfg.NodeName == "" {
		hostname, _ := os.Hostname()
		cfg.NodeName = hostname
	}
	if cfg.TunnelName == "" {
		cfg.TunnelName = "easyvpn0"
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
