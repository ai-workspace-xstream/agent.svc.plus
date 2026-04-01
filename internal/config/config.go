package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Mode  string `yaml:"mode"`
	Log   Log    `yaml:"log"`
	Agent Agent  `yaml:"agent"`
	Xray  Xray   `yaml:"xray"`
}

type Log struct {
	Level string `yaml:"level"`
}

type Agent struct {
	ID             string        `yaml:"id"`
	NodeID         string        `yaml:"nodeId"`
	Region         string        `yaml:"region"`
	LineCode       string        `yaml:"lineCode"`
	PricingGroup   string        `yaml:"pricingGroup"`
	StatsEnabled   bool          `yaml:"statsEnabled"`
	ControllerURL  string        `yaml:"controllerUrl"`
	APIToken       string        `yaml:"apiToken"`
	Domain         string        `yaml:"domain"`
	HTTPTimeout    time.Duration `yaml:"httpTimeout"`
	StatusInterval time.Duration `yaml:"statusInterval"`
	SyncInterval   time.Duration `yaml:"syncInterval"`
	TLS            TLS           `yaml:"tls"`
}

type TLS struct {
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

type Xray struct {
	Sync XraySync `yaml:"sync"`
}

type XraySync struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
	// Targets allows defining multiple Xray configuration files to sync.
	Targets []SyncTarget `yaml:"targets"`

	// Legacy fields for backward compatibility or simple single-target config
	OutputPath      string   `yaml:"outputPath"`
	TemplatePath    string   `yaml:"templatePath"`
	ValidateCommand []string `yaml:"validateCommand"`
	RestartCommand  []string `yaml:"restartCommand"`
}

type SyncTarget struct {
	Name            string   `yaml:"name"`
	OutputPath      string   `yaml:"outputPath"`
	TemplatePath    string   `yaml:"templatePath"`
	ValidateCommand []string `yaml:"validateCommand"`
	RestartCommand  []string `yaml:"restartCommand"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	return LoadReader(f)
}

func LoadReader(r io.Reader) (*Config, error) {
	var cfg Config
	if err := yaml.NewDecoder(r).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if authURL := os.Getenv("AuthUrl"); authURL != "" {
		cfg.Agent.ControllerURL = authURL
	}
	if token := os.Getenv("INTERNAL_SERVICE_TOKEN"); token != "" {
		cfg.Agent.APIToken = token
	}
	if domain := os.Getenv("DOMAIN"); domain != "" {
		cfg.Agent.Domain = domain
	}

	return &cfg, nil
}
