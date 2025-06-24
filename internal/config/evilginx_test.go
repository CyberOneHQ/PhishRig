package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateEvilginxConfig(t *testing.T) {
	cfg := EngagementConfig{
		Domain: DomainConfig{
			Phishing:    "login.test.com",
			RedirectURL: "https://example.com/",
		},
		Evilginx: EvilginxConfig{AutoCert: true},
	}

	result := GenerateEvilginxConfig(cfg, "1.2.3.4")

	if result.General.Domain != "login.test.com" {
		t.Errorf("domain = %q, want login.test.com", result.General.Domain)
	}
	if result.General.ExternalIP != "1.2.3.4" {
		t.Errorf("external IP = %q, want 1.2.3.4", result.General.ExternalIP)
	}
	if result.General.BindIPv4 != "0.0.0.0" {
		t.Errorf("bind IP = %q, want 0.0.0.0", result.General.BindIPv4)
	}
	if result.General.UnauthURL != "https://example.com/" {
		t.Errorf("unauth URL = %q, want https://example.com/", result.General.UnauthURL)
	}
	if !result.General.AutoCert {
		t.Error("autocert should be true")
	}
	if result.General.HTTPSPort != 443 {
		t.Errorf("HTTPS port = %d, want 443", result.General.HTTPSPort)
	}
}

func TestWriteEvilginxConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := EvilginxGeneratedConfig{
		General: EvilginxGeneral{
			Domain:     "test.com",
			ExternalIP: "1.2.3.4",
		},
	}

	if err := WriteEvilginxConfig(cfg, dir); err != nil {
		t.Fatalf("WriteEvilginxConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}

	var loaded EvilginxGeneratedConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parsing config.json: %v", err)
	}

	if loaded.General.Domain != "test.com" {
		t.Errorf("loaded domain = %q, want test.com", loaded.General.Domain)
	}
}

func TestGenerateEvilginxConfigWithPhishlet(t *testing.T) {
	cfg := EngagementConfig{
		Domain: DomainConfig{
			Phishing:    "login.test.com",
			RedirectURL: "https://example.com/",
		},
		Phishlet: PhishletConfig{
			Name:       "o365",
			Hostname:   "login.test.com",
			AutoEnable: true,
		},
		Evilginx: EvilginxConfig{AutoCert: true},
	}

	result := GenerateEvilginxConfig(cfg, "1.2.3.4")

	if result.Phishlets == nil {
		t.Fatal("phishlets should not be nil")
	}
	p, ok := result.Phishlets["o365"]
	if !ok {
		t.Fatal("o365 phishlet not found in config")
	}
	if p.Hostname != "login.test.com" {
		t.Errorf("phishlet hostname = %q, want login.test.com", p.Hostname)
	}
	if !p.Enabled {
		t.Error("phishlet should be enabled")
	}
	if !p.Visible {
		t.Error("phishlet should be visible")
	}
}

func TestGenerateSetupCommands(t *testing.T) {
	cfg := EngagementConfig{
		Domain: DomainConfig{
			Phishing:    "login.test.com",
			RedirectURL: "https://example.com/",
		},
		Phishlet: PhishletConfig{
			Name:     "o365",
			Hostname: "login.test.com",
		},
	}

	cmds := GenerateSetupCommands(cfg, "1.2.3.4")

	expected := []string{
		"config domain login.test.com",
		"config ipv4 external 1.2.3.4",
		"phishlets hostname o365 login.test.com",
		"phishlets enable o365",
		"lures create o365",
	}

	for _, exp := range expected {
		if !contains(cmds, exp) {
			t.Errorf("commands missing %q", exp)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
