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

func TestPublisherNewHomeAssistantConfig(t *testing.T) {
	pub, err := New(Config{
		Host: "127.0.0.1",
		HomeAssistant: &HomeAssistantConfig{
			Enabled:         true,
			DiscoveryPrefix: "homeassistant_custom",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !pub.haEnabled {
		t.Fatalf("haEnabled = false, want true")
	}
	if pub.haDiscoveryPrefix != "homeassistant_custom" {
		t.Fatalf("haDiscoveryPrefix = %q, want homeassistant_custom", pub.haDiscoveryPrefix)
	}
}

func TestHomeAssistantDiscoveryEntityNaming(t *testing.T) {
	check := model.CheckResult{
		NodeID:       "authentik",
		ServiceName:  "postgresql",
		ArtifactName: "postgresql",
	}

	title := check.ArtifactName
	if title == "" {
		title = check.ServiceName
	}

	updateConfig := haUpdateConfig{
		Name:       title + " Update",
		Title:      title,
		StateTopic: "up2date/authentik/postgresql/current_version",
	}

	sensorConfig := haSensorConfig{
		Name: title + " Check Status",
		Icon: "mdi:shield-search",
	}

	currentVersionConfig := haSensorConfig{
		Name:       title + " Current Version",
		StateTopic: "up2date/authentik/postgresql/current_version",
		Icon:       "mdi:tag-outline",
	}

	latestVersionConfig := haSensorConfig{
		Name:       title + " Latest Version",
		StateTopic: "up2date/authentik/postgresql/latest_version",
		Icon:       "mdi:tag-arrow-up-outline",
	}

	if updateConfig.Name != "postgresql Update" {
		t.Errorf("updateConfig.Name = %q, want %q", updateConfig.Name, "postgresql Update")
	}
	if updateConfig.StateTopic != "up2date/authentik/postgresql/current_version" {
		t.Errorf("updateConfig.StateTopic = %q, want %q", updateConfig.StateTopic, "up2date/authentik/postgresql/current_version")
	}
	if sensorConfig.Name != "postgresql Check Status" {
		t.Errorf("sensorConfig.Name = %q, want %q", sensorConfig.Name, "postgresql Check Status")
	}
	if sensorConfig.Icon != "mdi:shield-search" {
		t.Errorf("sensorConfig.Icon = %q, want %q", sensorConfig.Icon, "mdi:shield-search")
	}
	if currentVersionConfig.Name != "postgresql Current Version" {
		t.Errorf("currentVersionConfig.Name = %q, want %q", currentVersionConfig.Name, "postgresql Current Version")
	}
	if latestVersionConfig.Name != "postgresql Latest Version" {
		t.Errorf("latestVersionConfig.Name = %q, want %q", latestVersionConfig.Name, "postgresql Latest Version")
	}
}

