package mqtt

import (
	"reflect"
	"testing"

	"up2date/common/model"
)

func TestPublishedFieldsIncludeIdentityNames(t *testing.T) {
	check := model.CheckResult{
		ServiceName:      "paperless",
		ArtifactName:     "paperless-ngx",
		CurrentVersion:   "2.16.3",
		LatestVersion:    "2.17.0",
		LatestVersionURL: "https://example.invalid/tags?name=2.17.0",
		ObservedAt:       "2026-03-27T21:10:00Z",
		CheckStatus:      "outdated",
	}

	got := publishedFields(check)
	want := []publishedField{
		{name: "artifact_name", value: "paperless-ngx"},
		{name: "current_version", value: "2.16.3"},
		{name: "latest_version", value: "2.17.0"},
		{name: "latest_version_url", value: "https://example.invalid/tags?name=2.17.0"},
		{name: "observed_at", value: "2026-03-27T21:10:00Z"},
		{name: "check_status", value: "outdated"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected published fields: %#v", got)
	}
}
