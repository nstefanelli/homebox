package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLoadsPositionalYAMLConfig(t *testing.T) {
	configPath := writeConfigFile(t, `
mode: production
web:
  port: "8811"
auth:
  api_key_pepper: yaml-test-pepper-that-is-at-least-32-bytes
`)
	setArgs(t, configPath)

	cfg, err := New("test-build", "test description")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if cfg.Mode != ModeProduction {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeProduction)
	}
	if cfg.Web.Port != "8811" {
		t.Fatalf("Web.Port = %q, want %q", cfg.Web.Port, "8811")
	}
	if cfg.Auth.APIKeyPepper != "yaml-test-pepper-that-is-at-least-32-bytes" {
		t.Fatalf("Auth.APIKeyPepper was not loaded from YAML")
	}
	if cfg.Build != "test-build" || cfg.Desc != "test description" {
		t.Fatalf("Version = %#v, want supplied build metadata", cfg.Version)
	}
}

func TestNewEnvironmentAndFlagsOverrideYAML(t *testing.T) {
	configPath := writeConfigFile(t, `
mode: development
web:
  port: "8811"
`)
	t.Setenv("HBOX_MODE", ModeProduction)
	t.Setenv("HBOX_WEB_PORT", "8822")
	setArgs(t, "--web-port=8833", configPath)

	cfg, err := New("test-build", "test description")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if cfg.Mode != ModeProduction {
		t.Fatalf("Mode = %q, want environment override %q", cfg.Mode, ModeProduction)
	}
	if cfg.Web.Port != "8833" {
		t.Fatalf("Web.Port = %q, want command-line override %q", cfg.Web.Port, "8833")
	}
}

func TestNewAllowsMissingContainerDefaultConfig(t *testing.T) {
	setArgs(t, defaultConfigPath)

	cfg, err := New("test-build", "test description")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if cfg.Mode != ModeDevelopment {
		t.Fatalf("Mode = %q, want default %q", cfg.Mode, ModeDevelopment)
	}
	if cfg.Web.Port != "7745" {
		t.Fatalf("Web.Port = %q, want default %q", cfg.Web.Port, "7745")
	}
}

func TestNewRejectsMissingExplicitConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing.yml")
	setArgs(t, configPath)

	_, err := New("test-build", "test description")
	if err == nil {
		t.Fatal("New() error = nil, want missing config error")
	}
	if !strings.Contains(err.Error(), "reading config file") || !strings.Contains(err.Error(), configPath) {
		t.Fatalf("New() error = %q, want path-specific read error", err)
	}
}

func TestNewRejectsInvalidYAML(t *testing.T) {
	configPath := writeConfigFile(t, "web: [not: valid\n")
	setArgs(t, configPath)

	_, err := New("test-build", "test description")
	if err == nil {
		t.Fatal("New() error = nil, want YAML parse error")
	}
	if !strings.Contains(err.Error(), "parsing config file") || !strings.Contains(err.Error(), configPath) {
		t.Fatalf("New() error = %q, want path-specific parse error", err)
	}
}

func TestNewRejectsUnknownTopLevelYAMLField(t *testing.T) {
	configPath := writeConfigFile(t, "unknown_setting: true\n")
	setArgs(t, configPath)

	_, err := New("test-build", "test description")
	if err == nil {
		t.Fatal("New() error = nil, want unknown top-level field error")
	}
	if !strings.Contains(err.Error(), "field unknown_setting not found") {
		t.Fatalf("New() error = %q, want unknown top-level field detail", err)
	}
}

func TestNewRejectsUnknownNestedYAMLField(t *testing.T) {
	configPath := writeConfigFile(t, `
web:
  unknown_setting: true
`)
	setArgs(t, configPath)

	_, err := New("test-build", "test description")
	if err == nil {
		t.Fatal("New() error = nil, want unknown nested field error")
	}
	if !strings.Contains(err.Error(), "field unknown_setting not found") {
		t.Fatalf("New() error = %q, want unknown nested field detail", err)
	}
}

func TestNewRejectsMultipleYAMLDocuments(t *testing.T) {
	configPath := writeConfigFile(t, `
mode: production
---
mode: development
`)
	setArgs(t, configPath)

	_, err := New("test-build", "test description")
	if err == nil {
		t.Fatal("New() error = nil, want multiple-document error")
	}
	if !strings.Contains(err.Error(), "multiple YAML documents are not supported") {
		t.Fatalf("New() error = %q, want multiple-document detail", err)
	}
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func setArgs(t *testing.T, args ...string) {
	t.Helper()

	original := os.Args
	os.Args = append([]string{"homebox-test"}, args...)
	t.Cleanup(func() {
		os.Args = original
	})
}
