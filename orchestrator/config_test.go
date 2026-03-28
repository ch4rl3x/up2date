package orchestrator

import "testing"

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
