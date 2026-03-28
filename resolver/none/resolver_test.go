package none

import (
	"context"
	"testing"
	"time"

	"up2date/common/model"
)

func TestResolveBuildsUnknownResults(t *testing.T) {
	resolver := New()
	snapshot := model.NewSnapshot(
		model.Node{ID: "lxc-samba-01"},
		"package",
		time.Date(2026, time.March, 28, 12, 0, 0, 0, time.UTC),
		[]model.Observation{
			{
				ServiceName:          "samba",
				ArtifactType:         "os_package",
				ArtifactName:         "samba",
				ArtifactRef:          "dpkg:samba",
				CurrentVersion:       "2:4.19.5+dfsg-4",
				CurrentVersionSource: "dpkg-query",
				ObservedVia:          "local_package_manager",
			},
		},
	)

	results, err := resolver.Resolve(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Resolve() returned %d results, want 1", len(results))
	}

	result := results[0]
	if result.CheckStatus != "unknown" {
		t.Fatalf("check status = %q, want unknown", result.CheckStatus)
	}
	if result.Resolver != "none" {
		t.Fatalf("resolver = %q, want none", result.Resolver)
	}
}
