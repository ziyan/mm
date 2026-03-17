package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ServerProfile struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
	TeamID   string `json:"team_id,omitempty"`
	TeamName string `json:"team_name,omitempty"`
}

type Config struct {
	ActiveProfile string                   `json:"active_profile"`
	Profiles      map[string]ServerProfile `json:"profiles"`
}

func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mm")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mm")
}

func configPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func Load() (*Config, error) {
	config := &Config{Profiles: make(map[string]ServerProfile)}
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}
	if config.Profiles == nil {
		config.Profiles = make(map[string]ServerProfile)
	}
	return config, nil
}

func (self *Config) Save() error {
	directory := ConfigDir()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(self, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

func (self *Config) ActiveServer() (*ServerProfile, error) {
	if self.ActiveProfile == "" {
		return nil, fmt.Errorf("no active profile. Run: mm auth login")
	}
	profile, ok := self.Profiles[self.ActiveProfile]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", self.ActiveProfile)
	}
	return &profile, nil
}

func (self *Config) SetProfile(name string, profile ServerProfile) {
	profile.Name = name
	self.Profiles[name] = profile
	if self.ActiveProfile == "" {
		self.ActiveProfile = name
	}
}
