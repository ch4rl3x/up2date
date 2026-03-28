package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"up2date/common/model"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCollectUsesTCPEndpoint(t *testing.T) {
	collector, err := New(Config{Endpoint: "tcp://dockerproxy:2375"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	collector.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatalf("request method = %s, want GET", request.Method)
		}
		if request.URL.Scheme != "http" {
			t.Fatalf("request scheme = %q, want http", request.URL.Scheme)
		}
		if request.URL.Host != "dockerproxy:2375" {
			t.Fatalf("request host = %q, want dockerproxy:2375", request.URL.Host)
		}
		if request.URL.Path != "/containers/json" {
			t.Fatalf("request path = %q, want /containers/json", request.URL.Path)
		}
		if got := request.URL.Query().Get("all"); got != "1" {
			t.Fatalf("all query = %q, want 1", got)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`[{
			"Id":"1234567890abcdef",
			"Names":["/nginx"],
			"Image":"docker.io/library/nginx:1.27-alpine",
			"State":"running",
			"Status":"Up 10 minutes",
			"Labels":{"com.docker.compose.service":"nginx"}
		}]`)),
		}, nil
	})

	snapshot, err := collector.Collect(context.Background(), model.Node{ID: "docker-host-01"}, "docker")
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}
	if len(snapshot.Observations) != 1 {
		t.Fatalf("observation count = %d, want 1", len(snapshot.Observations))
	}
	if snapshot.Observations[0].ServiceName != "nginx" {
		t.Fatalf("service name = %q, want nginx", snapshot.Observations[0].ServiceName)
	}
	if snapshot.Observations[0].ArtifactRef != "docker.io/library/nginx:1.27-alpine" {
		t.Fatalf("artifact ref = %q, want docker image ref", snapshot.Observations[0].ArtifactRef)
	}
}

func TestNewRejectsUnsupportedEndpointScheme(t *testing.T) {
	_, err := New(Config{Endpoint: "ssh://dockerproxy:22"})
	if err == nil {
		t.Fatal("New() returned nil error, want unsupported endpoint error")
	}
	if !strings.Contains(err.Error(), `unsupported scheme "ssh"`) {
		t.Fatalf("error = %q, want unsupported scheme", err)
	}
}
