package dockerhub

import (
	"testing"

	"up2date/common/model"
)

func TestSelectArtifactReferencePrefersArtifactRef(t *testing.T) {
	observation := model.Observation{
		ArtifactName: "nginx",
		ArtifactRef:  "docker.io/library/nginx:1.29-alpine",
	}

	if got := selectArtifactReference(observation); got != observation.ArtifactRef {
		t.Fatalf("selectArtifactReference() = %q, want %q", got, observation.ArtifactRef)
	}
}

func TestParseDockerHubReferenceStripsTagAndDigest(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tagged image reference",
			input: "docker.io/library/nginx:1.29-alpine",
			want:  "library/nginx",
		},
		{
			name:  "digested image reference",
			input: "docker.io/paperless-ngx/paperless-ngx@sha256:abc123",
			want:  "paperless-ngx/paperless-ngx",
		},
		{
			name:  "short docker hub name",
			input: "nginx",
			want:  "library/nginx",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, err := parseDockerHubReference(test.input)
			if err != nil {
				t.Fatalf("parseDockerHubReference(%q) returned error: %v", test.input, err)
			}
			if ref.Repository != test.want {
				t.Fatalf("parseDockerHubReference(%q) repository = %q, want %q", test.input, ref.Repository, test.want)
			}
		})
	}
}

func TestBuildLatestVersionURL(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		tag        string
		want       string
	}{
		{
			name:       "official image uses underscore path",
			repository: "library/nginx",
			tag:        "1.29-alpine",
			want:       "https://hub.docker.com/_/nginx/tags?name=1.29-alpine",
		},
		{
			name:       "namespaced image uses repository path",
			repository: "paperless-ngx/paperless-ngx",
			tag:        "2.17.0",
			want:       "https://hub.docker.com/r/paperless-ngx/paperless-ngx/tags?name=2.17.0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := buildLatestVersionURL(test.repository, test.tag); got != test.want {
				t.Fatalf("buildLatestVersionURL(%q, %q) = %q, want %q", test.repository, test.tag, got, test.want)
			}
		})
	}
}
