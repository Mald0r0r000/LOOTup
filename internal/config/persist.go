package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// persistedConfig is the subset saved to disk
type persistedConfig struct {
	Host        string `json:"host"`
	User        string `json:"user"`
	KeyPath     string `json:"key_path"`
	DestPath    string `json:"dest_path"`
	Template    string `json:"template"`
	Concurrency int    `json:"concurrency"`
}

// configPath returns ~/.config/lootup/config.json
func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "lootup", "config.json")
}

// LoadPersisted reads saved config from disk. Returns empty values on error.
func LoadPersisted() (host, user, keyPath, destPath, tmpl string, concurrency int) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	var p persistedConfig
	if err := json.Unmarshal(data, &p); err != nil {
		return
	}
	return p.Host, p.User, p.KeyPath, p.DestPath, p.Template, p.Concurrency
}

// SavePersisted writes host config to disk for next launch
func SavePersisted(cfg *Config) error {
	p := persistedConfig{
		Host:        cfg.Host,
		User:        cfg.User,
		KeyPath:     cfg.KeyPath,
		DestPath:    cfg.DestPath,
		Template:    cfg.Template,
		Concurrency: cfg.Concurrency,
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(configPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(configPath(), data, 0644)
}
