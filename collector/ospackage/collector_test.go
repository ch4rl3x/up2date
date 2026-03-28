package ospackage

import (
	"context"
	"errors"
	"testing"

	"up2date/common/model"
)

type stubLookup struct {
	manager string
	source  string
	values  map[string]string
	errs    map[string]error
}

func (s stubLookup) Version(_ context.Context, packageName string) (string, error) {
	if err := s.errs[packageName]; err != nil {
		return "", err
	}
	return s.values[packageName], nil
}

func (s stubLookup) Manager() string {
	return s.manager
}

func (s stubLookup) Source() string {
	return s.source
}

func TestCollectBuildsPackageObservations(t *testing.T) {
	collector := &Collector{
		lookup: stubLookup{
			manager: "dpkg",
			source:  "dpkg-query",
			values: map[string]string{
				"samba":  "2:4.19.5+dfsg-4",
				"rsync":  "3.2.7-1",
				"unused": "0",
			},
		},
		names: []string{"samba", "rsync"},
	}

	snapshot, err := collector.Collect(context.Background(), model.Node{ID: "lxc-samba-01"}, "package")
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}

	if len(snapshot.Observations) != 2 {
		t.Fatalf("Collect() returned %d observations, want 2", len(snapshot.Observations))
	}

	first := snapshot.Observations[0]
	if first.ServiceName != "rsync" {
		t.Fatalf("first observation service = %q, want rsync", first.ServiceName)
	}
	if first.ArtifactType != "os_package" {
		t.Fatalf("artifact type = %q, want os_package", first.ArtifactType)
	}

	second := snapshot.Observations[1]
	if second.ServiceName != "samba" {
		t.Fatalf("second observation service = %q, want samba", second.ServiceName)
	}
	if second.CurrentVersion != "2:4.19.5+dfsg-4" {
		t.Fatalf("current version = %q, want package version", second.CurrentVersion)
	}
	if second.Attributes["installation_state"] != "installed" {
		t.Fatalf("installation_state = %q, want installed", second.Attributes["installation_state"])
	}
}

func TestCollectMarksMissingPackagesWithoutFailingTheSnapshot(t *testing.T) {
	collector := &Collector{
		lookup: stubLookup{
			manager: "dpkg",
			source:  "dpkg-query",
			errs: map[string]error{
				"samba": errPackageNotInstalled,
			},
		},
		names: []string{"samba"},
	}

	snapshot, err := collector.Collect(context.Background(), model.Node{ID: "lxc-samba-01"}, "package")
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}

	observation := snapshot.Observations[0]
	if observation.CurrentVersion != "" {
		t.Fatalf("current version = %q, want empty", observation.CurrentVersion)
	}
	if observation.CurrentVersionSource != "dpkg-query" {
		t.Fatalf("current version source = %q, want dpkg-query", observation.CurrentVersionSource)
	}
	if observation.Attributes["installation_state"] != "not_installed" {
		t.Fatalf("installation_state = %q, want not_installed", observation.Attributes["installation_state"])
	}
}

func TestCollectFailsForUnexpectedLookupErrors(t *testing.T) {
	collector := &Collector{
		lookup: stubLookup{
			manager: "dpkg",
			source:  "dpkg-query",
			errs: map[string]error{
				"samba": errors.New("boom"),
			},
		},
		names: []string{"samba"},
	}

	if _, err := collector.Collect(context.Background(), model.Node{ID: "lxc-samba-01"}, "package"); err == nil {
		t.Fatalf("Collect() returned nil error, want failure")
	}
}

func TestNewVersionLookupSupportsBrewAliases(t *testing.T) {
	tests := []struct {
		name    string
		manager string
		want    string
	}{
		{
			name:    "brew short name",
			manager: "brew",
			want:    "brew",
		},
		{
			name:    "homebrew alias",
			manager: "homebrew",
			want:    "brew",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup, err := newVersionLookup(test.manager)
			if err != nil {
				t.Fatalf("newVersionLookup(%q) returned error: %v", test.manager, err)
			}
			if got := lookup.Manager(); got != test.want {
				t.Fatalf("lookup manager = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseBrewInfoVersion(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		packageName string
		want        string
		installed   bool
	}{
		{
			name: "single installed version",
			output: `{
				"formulae": [
					{
						"name": "samba",
						"installed": [{"version": "4.22.3"}]
					}
				]
			}`,
			packageName: "samba",
			want:        "4.22.3",
			installed:   true,
		},
		{
			name: "multiple installed versions keep newest entry",
			output: `{
				"formulae": [
					{
						"name": "python@3.12",
						"installed": [{"version": "3.12.11"}, {"version": "3.12.12"}]
					}
				]
			}`,
			packageName: "python@3.12",
			want:        "3.12.12",
			installed:   true,
		},
		{
			name: "known formula but not installed",
			output: `{
				"formulae": [
					{
						"name": "samba",
						"installed": []
					}
				]
			}`,
			packageName: "samba",
			installed:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, installed, err := parseBrewInfoVersion([]byte(test.output), test.packageName)
			if err != nil {
				t.Fatalf("parseBrewInfoVersion() returned error: %v", err)
			}
			if installed != test.installed {
				t.Fatalf("installed = %t, want %t", installed, test.installed)
			}
			if got != test.want {
				t.Fatalf("parseBrewInfoVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseBrewInfoVersionRejectsUnexpectedOutput(t *testing.T) {
	if _, _, err := parseBrewInfoVersion([]byte("4.22.3\n"), "samba"); err == nil {
		t.Fatalf("parseBrewInfoVersion() returned nil error for malformed output")
	}
}
