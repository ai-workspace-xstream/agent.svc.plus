package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Mode    string  `yaml:"mode"`
	Log     Log     `yaml:"log"`
	Agent   Agent   `yaml:"agent"`
	Xray    Xray    `yaml:"xray"`
	Caddy   Caddy   `yaml:"caddy"`
	Billing Billing `yaml:"billing"`
}

// Caddy configures the embedded (in-process) Caddy instance. When Embedded
// is true, the supervisor runs Caddy as a library goroutine instead of
// shelling out to a separate caddy process/systemd unit. Left zero, the
// agent keeps the legacy external-process behaviour.
type Caddy struct {
	Embedded     bool   `yaml:"embedded"`
	DNSProvider  string `yaml:"dnsProvider"`  // "cloudflare" | "alidns"
	DNSAPIToken  string `yaml:"dnsApiToken"`  // cloudflare
	AliKeyID     string `yaml:"aliKeyId"`     // alidns
	AliKeySecret string `yaml:"aliKeySecret"` // alidns
	XHTTPSocket  string `yaml:"xhttpSocket"`  // unix socket shared with embedded xhttp Xray
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
	// Embedded runs Xray inbounds as in-process core.Instances instead of
	// writing config files and restarting an external xray service. Sync
	// targets still describe the inbounds; their ValidateCommand /
	// RestartCommand are ignored in embedded mode.
	Embedded bool     `yaml:"embedded"`
	Sync     XraySync `yaml:"sync"`
}

type Billing struct {
	Enabled           bool          `yaml:"enabled"`
	BaseURL           string        `yaml:"baseURL"`
	HTTPTimeout       time.Duration `yaml:"httpTimeout"`
	CollectInterval   time.Duration `yaml:"collectInterval"`
	ReconcileInterval time.Duration `yaml:"reconcileInterval"`
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
	if billingURL := os.Getenv("BILLING_SERVICE_BASE_URL"); billingURL != "" {
		cfg.Billing.BaseURL = billingURL
	}

	return &cfg, nil
}
