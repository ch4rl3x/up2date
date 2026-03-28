package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"up2date/common/model"
)

func TestParseRegistryReferenceStripsTagAndDigest(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantKind       string
		wantRegistry   string
		wantRepository string
	}{
		{
			name:           "docker hub short name",
			input:          "nginx:1.29-alpine",
			wantKind:       registryKindDockerHub,
			wantRegistry:   "docker.io",
			wantRepository: "library/nginx",
		},
		{
			name:           "docker hub explicit registry",
			input:          "registry-1.docker.io/library/postgres@sha256:abc123",
			wantKind:       registryKindDockerHub,
			wantRegistry:   "docker.io",
			wantRepository: "library/postgres",
		},
		{
			name:           "ghcr image",
			input:          "ghcr.io/goauthentik/server:2026.2.1",
			wantKind:       registryKindGHCR,
			wantRegistry:   "ghcr.io",
			wantRepository: "goauthentik/server",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, err := parseRegistryReference(test.input)
			if err != nil {
				t.Fatalf("parseRegistryReference(%q) returned error: %v", test.input, err)
			}
			if ref.Kind != test.wantKind {
				t.Fatalf("parseRegistryReference(%q) kind = %q, want %q", test.input, ref.Kind, test.wantKind)
			}
			if ref.Registry != test.wantRegistry {
				t.Fatalf("parseRegistryReference(%q) registry = %q, want %q", test.input, ref.Registry, test.wantRegistry)
			}
			if ref.Repository != test.wantRepository {
				t.Fatalf("parseRegistryReference(%q) repository = %q, want %q", test.input, ref.Repository, test.wantRepository)
			}
		})
	}
}

func TestParseRegistryReferenceRejectsUnsupportedRegistry(t *testing.T) {
	_, err := parseRegistryReference("quay.io/keycloak/keycloak:26.2")
	if err == nil {
		t.Fatal("parseRegistryReference() returned nil error, want unsupported registry error")
	}
	if !strings.Contains(err.Error(), `does not support registry "quay.io"`) {
		t.Fatalf("error = %q, want unsupported registry", err)
	}
}

func TestBuildLatestVersionURL(t *testing.T) {
	tests := []struct {
		name string
		ref  registryReference
		tag  string
		want string
	}{
		{
			name: "official docker hub image uses underscore path",
			ref: registryReference{
				Kind:       registryKindDockerHub,
				Repository: "library/nginx",
			},
			tag:  "1.29-alpine",
			want: "https://hub.docker.com/_/nginx/tags?name=1.29-alpine",
		},
		{
			name: "namespaced docker hub image uses repository path",
			ref: registryReference{
				Kind:       registryKindDockerHub,
				Repository: "paperless-ngx/paperless-ngx",
			},
			tag:  "2.17.0",
			want: "https://hub.docker.com/r/paperless-ngx/paperless-ngx/tags?name=2.17.0",
		},
		{
			name: "ghcr currently leaves latest version url empty",
			ref: registryReference{
				Kind:       registryKindGHCR,
				Repository: "goauthentik/server",
			},
			tag:  "2026.2.1",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := buildLatestVersionURL(test.ref, test.tag); got != test.want {
				t.Fatalf("buildLatestVersionURL(%+v, %q) = %q, want %q", test.ref, test.tag, got, test.want)
			}
		})
	}
}

func TestDetermineResolutionPrefersDeployedImageTagOverContainerLabel(t *testing.T) {
	observation := model.Observation{
		ServiceName:    "mosquitto",
		ArtifactName:   "eclipse-mosquitto",
		ArtifactRef:    "docker.io/eclipse-mosquitto:2.1.2-alpine",
		CurrentVersion: "2.0.22",
		ObservedVia:    "docker_socket",
		Attributes: map[string]string{
			"image_tag":         "2.1.2-alpine",
			"version_label_key": "org.opencontainers.image.version",
			"container_name":    "mosquitto",
		},
	}
	ref := registryReference{
		Kind:       registryKindDockerHub,
		Registry:   "docker.io",
		Repository: "eclipse-mosquitto",
	}

	result := determineResolution(observation, ref, []string{
		"2.0.22",
		"2.1.2-alpine",
		"2.1.3-alpine",
	})

	if result.status != "outdated" {
		t.Fatalf("status = %q, want outdated", result.status)
	}
	if result.latestVersion != "2.1.3-alpine" {
		t.Fatalf("latest version = %q, want 2.1.3-alpine", result.latestVersion)
	}
	if result.update == nil || !*result.update {
		t.Fatalf("update available = %#v, want true", result.update)
	}
}

func TestRegistryClientListTagsHandlesBearerChallengeAndPagination(t *testing.T) {
	tokenRequests := 0
	tagRequests := 0

	client := registryClient{httpClient: &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.URL.Host == "ghcr.example.test" && request.URL.Path == "/token":
				tokenRequests++
				if got := request.URL.Query().Get("service"); got != "ghcr.io" {
					t.Fatalf("token service = %q, want ghcr.io", got)
				}
				if got := request.URL.Query().Get("scope"); got != "repository:goauthentik/server:pull" {
					t.Fatalf("token scope = %q, want repository pull scope", got)
				}
				return jsonResponse(request, http.StatusOK, nil, `{"token":"ghcr-token"}`), nil
			case request.URL.Host == "ghcr.example.test" && request.URL.Path == "/v2/goauthentik/server/tags/list":
				tagRequests++
				if request.Header.Get("Authorization") == "" {
					return jsonResponse(request, http.StatusUnauthorized, map[string]string{
						"WWW-Authenticate": `Bearer realm="https://ghcr.example.test/token",service="ghcr.io",scope="repository:goauthentik/server:pull"`,
					}, ""), nil
				}
				if got := request.Header.Get("Authorization"); got != "Bearer ghcr-token" {
					t.Fatalf("authorization = %q, want bearer token", got)
				}
				if request.URL.Query().Get("last") == "" {
					return jsonResponse(request, http.StatusOK, map[string]string{
						"Link": `</v2/goauthentik/server/tags/list?n=1000&last=2026.2.0>; rel="next"`,
					}, `{"tags":["2026.2.0"]}`), nil
				}
				return jsonResponse(request, http.StatusOK, nil, `{"tags":["2026.2.1"]}`), nil
			default:
				t.Fatalf("unexpected request to %s", request.URL.String())
				return nil, nil
			}
		}),
	}}
	ref := registryReference{
		Kind:       registryKindGHCR,
		Registry:   "ghcr.io",
		Repository: "goauthentik/server",
		APIBaseURL: "https://ghcr.example.test",
	}

	tags, err := client.listTags(context.Background(), ref)
	if err != nil {
		t.Fatalf("listTags() returned error: %v", err)
	}
	if len(tags) != 2 || tags[0] != "2026.2.0" || tags[1] != "2026.2.1" {
		t.Fatalf("tags = %v, want [2026.2.0 2026.2.1]", tags)
	}
	if tokenRequests != 1 {
		t.Fatalf("token requests = %d, want 1", tokenRequests)
	}
	if tagRequests != 3 {
		t.Fatalf("tag requests = %d, want 3", tagRequests)
	}
}

func TestResolveRoutesMixedRegistries(t *testing.T) {
	resolver := New()
	resolver.client.httpClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.URL.Host == "registry-1.docker.io" && request.URL.Path == "/v2/library/nginx/tags/list":
				if request.Header.Get("Authorization") == "" {
					return jsonResponse(request, http.StatusUnauthorized, map[string]string{
						"WWW-Authenticate": `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"`,
					}, ""), nil
				}
				if got := request.Header.Get("Authorization"); got != "Bearer docker-token" {
					t.Fatalf("docker hub authorization = %q, want docker token", got)
				}
				return jsonResponse(request, http.StatusOK, nil, `{"tags":["1.27-alpine","1.29-alpine"]}`), nil
			case request.URL.Host == "auth.docker.io" && request.URL.Path == "/token":
				return jsonResponse(request, http.StatusOK, nil, `{"token":"docker-token"}`), nil
			case request.URL.Host == "ghcr.io" && request.URL.Path == "/v2/goauthentik/server/tags/list":
				if request.Header.Get("Authorization") == "" {
					return jsonResponse(request, http.StatusUnauthorized, map[string]string{
						"WWW-Authenticate": `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:goauthentik/server:pull"`,
					}, ""), nil
				}
				if got := request.Header.Get("Authorization"); got != "Bearer ghcr-token" {
					t.Fatalf("ghcr authorization = %q, want ghcr token", got)
				}
				return jsonResponse(request, http.StatusOK, nil, `{"tags":["2026.2.0","2026.2.1"]}`), nil
			case request.URL.Host == "ghcr.io" && request.URL.Path == "/token":
				return jsonResponse(request, http.StatusOK, nil, `{"token":"ghcr-token"}`), nil
			default:
				t.Fatalf("unexpected request to %s", request.URL.String())
				return nil, nil
			}
		}),
	}

	observedAt, err := time.Parse(time.RFC3339, "2026-03-28T10:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse() returned error: %v", err)
	}

	snapshot := model.NewSnapshot(model.Node{ID: "docker-host-01"}, "docker", observedAt, []model.Observation{
		{
			ServiceName:  "nginx",
			ArtifactType: "container_image",
			ArtifactName: "nginx",
			ArtifactRef:  "docker.io/library/nginx:1.27-alpine",
			ObservedVia:  "docker_socket",
			Attributes: map[string]string{
				"image_tag": "1.27-alpine",
			},
		},
		{
			ServiceName:  "authentik-server",
			ArtifactType: "container_image",
			ArtifactName: "server",
			ArtifactRef:  "ghcr.io/goauthentik/server:2026.2.0",
			ObservedVia:  "docker_socket",
			Attributes: map[string]string{
				"image_tag": "2026.2.0",
			},
		},
		{
			ServiceName:  "keycloak",
			ArtifactType: "container_image",
			ArtifactName: "keycloak",
			ArtifactRef:  "quay.io/keycloak/keycloak:26.2",
			ObservedVia:  "docker_socket",
			Attributes: map[string]string{
				"image_tag": "26.2",
			},
		},
	})

	results, err := resolver.Resolve(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results count = %d, want 3", len(results))
	}

	byService := make(map[string]model.CheckResult, len(results))
	for _, result := range results {
		byService[result.ServiceName] = result
	}

	nginx := byService["nginx"]
	if nginx.Resolver != "docker" {
		t.Fatalf("nginx resolver = %q, want docker", nginx.Resolver)
	}
	if nginx.CheckStatus != "outdated" {
		t.Fatalf("nginx status = %q, want outdated", nginx.CheckStatus)
	}
	if nginx.LatestVersion != "1.29-alpine" {
		t.Fatalf("nginx latest version = %q, want 1.29-alpine", nginx.LatestVersion)
	}
	if nginx.LatestVersionURL != "https://hub.docker.com/_/nginx/tags?name=1.29-alpine" {
		t.Fatalf("nginx latest version url = %q, want docker hub url", nginx.LatestVersionURL)
	}

	authentik := byService["authentik-server"]
	if authentik.CheckStatus != "outdated" {
		t.Fatalf("authentik status = %q, want outdated", authentik.CheckStatus)
	}
	if authentik.LatestVersion != "2026.2.1" {
		t.Fatalf("authentik latest version = %q, want 2026.2.1", authentik.LatestVersion)
	}
	if authentik.LatestVersionURL != "" {
		t.Fatalf("authentik latest version url = %q, want empty", authentik.LatestVersionURL)
	}

	keycloak := byService["keycloak"]
	if keycloak.CheckStatus != "unsupported" {
		t.Fatalf("keycloak status = %q, want unsupported", keycloak.CheckStatus)
	}
	if !strings.Contains(keycloak.Reason, `does not support registry "quay.io"`) {
		t.Fatalf("keycloak reason = %q, want unsupported registry reason", keycloak.Reason)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(request *http.Request, status int, headers map[string]string, body string) *http.Response {
	response := &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
	for key, value := range headers {
		response.Header.Set(key, value)
	}
	return response
}
