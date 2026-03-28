package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPackageCollectorDefaultsToNoneResolver(t *testing.T) {
	t.Setenv("UP2DATE_NODE_ID", "lxc-samba-01")
	t.Setenv("UP2DATE_INTERVAL", "1m")
	t.Setenv("UP2DATE_COLLECTOR_TYPE", "package")
	t.Setenv("UP2DATE_COLLECTOR_PACKAGE_NAMES", "samba, wsdd2")
	t.Setenv("UP2DATE_PUBLISHER_TYPE", "mqtt")
	t.Setenv("UP2DATE_PUBLISHER_MQTT_HOST", "mqtt")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Job.Resolver.Type != "none" {
		t.Fatalf("resolver type = %q, want none", cfg.Job.Resolver.Type)
	}
	if len(cfg.Job.Collector.Package.Names) != 2 {
		t.Fatalf("package names count = %d, want 2", len(cfg.Job.Collector.Package.Names))
	}
	if cfg.Job.Collector.Package.Names[0] != "samba" {
		t.Fatalf("first package name = %q, want samba", cfg.Job.Collector.Package.Names[0])
	}
	if cfg.Job.Collector.Package.Names[1] != "wsdd2" {
		t.Fatalf("second package name = %q, want wsdd2", cfg.Job.Collector.Package.Names[1])
	}
}

func TestLoadPackageCollectorDefaultsToBrewFormulaResolver(t *testing.T) {
	t.Setenv("UP2DATE_NODE_ID", "macbook-alex")
	t.Setenv("UP2DATE_INTERVAL", "1m")
	t.Setenv("UP2DATE_COLLECTOR_TYPE", "package")
	t.Setenv("UP2DATE_COLLECTOR_PACKAGE_MANAGER", "brew")
	t.Setenv("UP2DATE_COLLECTOR_PACKAGE_NAMES", "ripgrep")
	t.Setenv("UP2DATE_PUBLISHER_TYPE", "mqtt")
	t.Setenv("UP2DATE_PUBLISHER_MQTT_HOST", "mqtt")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Job.Resolver.Type != "brew_formula" {
		t.Fatalf("resolver type = %q, want brew_formula", cfg.Job.Resolver.Type)
	}
}

func TestLoadFromFileDerivesResolverFromCollector(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "up2date.yaml")
	config := strings.TrimSpace(`
node_id: macbook-alex
interval: 5m
job_name: package-audit

collector:
  type: package
  package:
    manager: brew
    names:
      - samba
      - ripgrep

publisher:
  mqtt:
    host: 127.0.0.1
    port: 1883
    retain: true
`)

	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() returned error: %v", err)
	}

	if cfg.Node.ID != "macbook-alex" {
		t.Fatalf("node id = %q, want macbook-alex", cfg.Node.ID)
	}
	if cfg.Job.Name != "package-audit" {
		t.Fatalf("job name = %q, want package-audit", cfg.Job.Name)
	}
	if cfg.Job.Interval != 5*time.Minute {
		t.Fatalf("interval = %s, want 5m", cfg.Job.Interval)
	}
	if cfg.Job.Resolver.Type != "brew_formula" {
		t.Fatalf("resolver type = %q, want brew_formula", cfg.Job.Resolver.Type)
	}
	if cfg.Job.Publisher.Type != "mqtt" {
		t.Fatalf("publisher type = %q, want mqtt", cfg.Job.Publisher.Type)
	}
	if cfg.Job.Publisher.MQTT.Port != 1883 {
		t.Fatalf("mqtt port = %d, want 1883", cfg.Job.Publisher.MQTT.Port)
	}
	if cfg.Job.Publisher.MQTT.Retain == nil || !*cfg.Job.Publisher.MQTT.Retain {
		t.Fatalf("mqtt retain = %v, want true", cfg.Job.Publisher.MQTT.Retain)
	}
	if len(cfg.Job.Collector.Package.Names) != 2 {
		t.Fatalf("package names count = %d, want 2", len(cfg.Job.Collector.Package.Names))
	}
}

func TestLoadFromFileRejectsResolverBlock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "up2date.yaml")
	config := strings.TrimSpace(`
collector:
  type: docker

resolver:
  type: none

publisher:
  mqtt:
    host: mqtt
`)

	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	_, err := LoadFromFile(configPath)
	if err == nil {
		t.Fatal("LoadFromFile() returned nil error, want resolver rejection")
	}
	if !strings.Contains(err.Error(), "resolver is not configurable") {
		t.Fatalf("error = %q, want resolver rejection", err)
	}
}
