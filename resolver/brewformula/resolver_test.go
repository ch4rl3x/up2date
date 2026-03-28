package brewformula

import (
	"context"
	"errors"
	"testing"
	"time"

	"up2date/common/model"
)

type stubClient struct {
	infoValues     map[string]infoFormula
	infoErrs       map[string]error
	outdatedValues map[string]outdatedFormula
	outdatedFound  map[string]bool
	outdatedErrs   map[string]error
}

func (s stubClient) info(_ context.Context, formulaName string) (infoFormula, error) {
	if err := s.infoErrs[formulaName]; err != nil {
		return infoFormula{}, err
	}
	return s.infoValues[formulaName], nil
}

func (s stubClient) outdated(_ context.Context, formulaName string) (outdatedFormula, bool, error) {
	if err := s.outdatedErrs[formulaName]; err != nil {
		return outdatedFormula{}, false, err
	}
	return s.outdatedValues[formulaName], s.outdatedFound[formulaName], nil
}

func TestResolveCurrentBrewFormula(t *testing.T) {
	resolver := &Resolver{
		client: stubClient{
			infoValues: map[string]infoFormula{
				"go": {
					Name: "go",
					Installed: []infoInstalled{
						{Version: "1.26.1"},
					},
					Outdated: false,
				},
			},
		},
	}

	results, err := resolver.Resolve(context.Background(), snapshotForObservation(model.Observation{
		ServiceName:          "go",
		ArtifactType:         "os_package",
		ArtifactName:         "go",
		ArtifactRef:          "brew:go",
		CurrentVersion:       "1.26.1",
		CurrentVersionSource: "brew info --json=v2",
		ObservedVia:          "local_package_manager",
		Attributes: map[string]string{
			"package_manager": "brew",
			"package_name":    "go",
		},
	}))
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	result := results[0]
	if result.CheckStatus != "current" {
		t.Fatalf("check status = %q, want current", result.CheckStatus)
	}
	if result.LatestVersion != "1.26.1" {
		t.Fatalf("latest version = %q, want 1.26.1", result.LatestVersion)
	}
	if result.UpdateAvailable == nil || *result.UpdateAvailable {
		t.Fatalf("update available = %#v, want false", result.UpdateAvailable)
	}
}

func TestResolveOutdatedBrewFormula(t *testing.T) {
	resolver := &Resolver{
		client: stubClient{
			infoValues: map[string]infoFormula{
				"gettext": {
					Name: "gettext",
					Installed: []infoInstalled{
						{Version: "0.26_1"},
					},
					Outdated: true,
				},
			},
			outdatedValues: map[string]outdatedFormula{
				"gettext": {
					Name:              "gettext",
					InstalledVersions: []string{"0.26_1"},
					CurrentVersion:    "1.0",
				},
			},
			outdatedFound: map[string]bool{
				"gettext": true,
			},
		},
	}

	results, err := resolver.Resolve(context.Background(), snapshotForObservation(model.Observation{
		ServiceName:          "gettext",
		ArtifactType:         "os_package",
		ArtifactName:         "gettext",
		ArtifactRef:          "brew:gettext",
		CurrentVersion:       "0.26_1",
		CurrentVersionSource: "brew info --json=v2",
		ObservedVia:          "local_package_manager",
		Attributes: map[string]string{
			"package_manager": "brew",
			"package_name":    "gettext",
		},
	}))
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	result := results[0]
	if result.CheckStatus != "outdated" {
		t.Fatalf("check status = %q, want outdated", result.CheckStatus)
	}
	if result.LatestVersion != "1.0" {
		t.Fatalf("latest version = %q, want 1.0", result.LatestVersion)
	}
	if result.UpdateAvailable == nil || !*result.UpdateAvailable {
		t.Fatalf("update available = %#v, want true", result.UpdateAvailable)
	}
}

func TestResolveUnsupportedObservation(t *testing.T) {
	resolver := New()

	results, err := resolver.Resolve(context.Background(), snapshotForObservation(model.Observation{
		ServiceName:  "samba",
		ArtifactType: "os_package",
		ArtifactName: "samba",
		ArtifactRef:  "dpkg:samba",
		ObservedVia:  "local_package_manager",
		Attributes: map[string]string{
			"package_manager": "dpkg",
			"package_name":    "samba",
		},
	}))
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	if results[0].CheckStatus != "unsupported" {
		t.Fatalf("check status = %q, want unsupported", results[0].CheckStatus)
	}
}

func TestResolvePropagatesInfoErrorsAsCheckErrors(t *testing.T) {
	resolver := &Resolver{
		client: stubClient{
			infoErrs: map[string]error{
				"go": errors.New("boom"),
			},
		},
	}

	results, err := resolver.Resolve(context.Background(), snapshotForObservation(model.Observation{
		ServiceName:  "go",
		ArtifactName: "go",
		ArtifactRef:  "brew:go",
		ObservedVia:  "local_package_manager",
		Attributes:   map[string]string{"package_manager": "brew", "package_name": "go"},
	}))
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	if results[0].CheckStatus != "error" {
		t.Fatalf("check status = %q, want error", results[0].CheckStatus)
	}
}

func snapshotForObservation(observation model.Observation) model.Snapshot {
	return model.NewSnapshot(
		model.Node{ID: "macbook-alex"},
		"package",
		time.Date(2026, time.March, 28, 12, 0, 0, 0, time.UTC),
		[]model.Observation{observation},
	)
}
