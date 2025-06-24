package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EvilginxGeneratedConfig represents the config.json that Evilginx v3.3.0 reads
type EvilginxGeneratedConfig struct {
	Blacklist EvilginxBlacklist            `json:"blacklist"`
	General   EvilginxGeneral              `json:"general"`
	Phishlets map[string]EvilginxPhishlet  `json:"phishlets,omitempty"`
}

// EvilginxGeneral matches Evilginx v3.3.0's actual config format
type EvilginxGeneral struct {
	AutoCert   bool   `json:"autocert"`
	BindIPv4   string `json:"bind_ipv4"`
	DNSPort    int    `json:"dns_port"`
	Domain     string `json:"domain"`
	ExternalIP string `json:"external_ipv4"`
	HTTPSPort  int    `json:"https_port"`
	IPv4       string `json:"ipv4"`
	UnauthURL  string `json:"unauth_url"`
}

type EvilginxBlacklist struct {
	Mode string `json:"mode"`
}

type EvilginxPhishlet struct {
	Hostname  string `json:"hostname"`
	UnauthURL string `json:"unauth_url"`
	Enabled   bool   `json:"enabled"`
	Visible   bool   `json:"visible"`
}

// GenerateEvilginxConfig creates config.json for Evilginx v3.3.0
func GenerateEvilginxConfig(cfg EngagementConfig, publicIP string) EvilginxGeneratedConfig {
	egCfg := EvilginxGeneratedConfig{
		General: EvilginxGeneral{
			AutoCert:   cfg.Evilginx.AutoCert,
			BindIPv4:   "0.0.0.0",
			DNSPort:    53,
			Domain:     cfg.Domain.Phishing,
			ExternalIP: publicIP,
			HTTPSPort:  443,
			IPv4:       "",
			UnauthURL:  cfg.Domain.RedirectURL,
		},
		Blacklist: EvilginxBlacklist{
			Mode: "unauth",
		},
	}

	// Configure phishlet hostname and enable state
	if cfg.Phishlet.Name != "" {
		egCfg.Phishlets = map[string]EvilginxPhishlet{
			cfg.Phishlet.Name: {
				Hostname:  cfg.Phishlet.Hostname,
				UnauthURL: "",
				Enabled:   cfg.Phishlet.AutoEnable,
				Visible:   true,
			},
		}
	}

	return egCfg
}

// WriteEvilginxConfig writes the generated config to the Evilginx config directory
func WriteEvilginxConfig(cfg EvilginxGeneratedConfig, dataDir string) error {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return fmt.Errorf("creating evilginx data dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling evilginx config: %w", err)
	}

	path := filepath.Join(dataDir, "config.json")
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("writing evilginx config to %s: %w", path, err)
	}

	return nil
}

// GenerateSetupCommands returns the Evilginx v3.3.0 interactive commands
func GenerateSetupCommands(cfg EngagementConfig, publicIP string) string {
	return fmt.Sprintf(`config domain %s
config ipv4 external %s
config ipv4 bind %s
phishlets hostname %s %s
phishlets enable %s
lures create %s`,
		cfg.Domain.Phishing,
		publicIP,
		publicIP,
		cfg.Phishlet.Name,
		cfg.Phishlet.Hostname,
		cfg.Phishlet.Name,
		cfg.Phishlet.Name,
	)
}
