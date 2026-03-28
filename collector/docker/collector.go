package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"up2date/common/model"
)

const (
	defaultSocket        = "/var/run/docker.sock"
	defaultDockerAPIHost = "http://docker"
)

var (
	defaultExcludeLabels = []string{
		"up2date.ignore=true",
		"com.up2date.ignore=true",
	}
	selfContainerIDPatterns = []*regexp.Regexp{
		regexp.MustCompile(`libpod-([a-f0-9]{12,64})(?:\.scope)?`),
		regexp.MustCompile(`docker-([a-f0-9]{12,64})(?:\.scope)?`),
		regexp.MustCompile(`/containers/([a-f0-9]{12,64})/`),
	}
	versionLabelKeys = []string{
		"org.opencontainers.image.version",
		"org.label-schema.version",
	}
)

type Config struct {
	Endpoint       string   `json:"endpoint,omitempty"`
	IncludeStopped *bool    `json:"include_stopped,omitempty"`
	ExcludeSelf    *bool    `json:"exclude_self,omitempty"`
	ExcludeLabels  []string `json:"exclude_labels,omitempty"`
}

type Collector struct {
	endpoint       string
	endpointScheme string
	baseURL        string
	includeStopped bool
	excludeSelf    bool
	excludeLabels  []string
	client         *http.Client
}

type container struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

type endpointConfig struct {
	display string
	scheme  string
	baseURL string
	socket  string
}

func New(cfg Config) (*Collector, error) {
	includeStopped := true
	if cfg.IncludeStopped != nil {
		includeStopped = *cfg.IncludeStopped
	}

	excludeSelf := true
	if cfg.ExcludeSelf != nil {
		excludeSelf = *cfg.ExcludeSelf
	}

	excludeLabels := append([]string(nil), cfg.ExcludeLabels...)
	if len(excludeLabels) == 0 {
		excludeLabels = append(excludeLabels, defaultExcludeLabels...)
	}

	endpoint, err := resolveEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if endpoint.scheme == "unix" {
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", endpoint.socket)
		}
	}

	return &Collector{
		endpoint:       endpoint.display,
		endpointScheme: endpoint.scheme,
		baseURL:        endpoint.baseURL,
		includeStopped: includeStopped,
		excludeSelf:    excludeSelf,
		excludeLabels:  excludeLabels,
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}, nil
}

func (i *Collector) Collect(ctx context.Context, node model.Node, jobName string) (model.Snapshot, error) {
	containers, err := i.listContainers(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}

	selfMatchers := []string(nil)
	if i.excludeSelf {
		selfMatchers = detectSelfMatchers()
	}

	observations := make([]model.Observation, 0, len(containers))
	for _, item := range containers {
		if shouldExclude(item, selfMatchers, i.excludeLabels) {
			continue
		}
		observations = append(observations, buildObservation(item))
	}

	sort.Slice(observations, func(left, right int) bool {
		a := observations[left]
		b := observations[right]
		projectA := a.Attributes["project_name"]
		projectB := b.Attributes["project_name"]
		if projectA != projectB {
			return projectA < projectB
		}
		if a.ServiceName != b.ServiceName {
			return a.ServiceName < b.ServiceName
		}
		return a.Attributes["container_name"] < b.Attributes["container_name"]
	})

	return model.NewSnapshot(node, jobName, time.Now().UTC(), observations), nil
}

func (i *Collector) listContainers(ctx context.Context) ([]container, error) {
	allValue := "0"
	if i.includeStopped {
		allValue = "1"
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.baseURL+"/containers/json?all="+allValue, nil)
	if err != nil {
		return nil, fmt.Errorf("build docker request: %w", err)
	}

	response, err := i.client.Do(request)
	if err != nil {
		if i.endpointScheme == "unix" && isPermissionDenied(err) {
			return nil, fmt.Errorf(
				"query docker socket %q: permission denied; mount the container host engine socket to /var/run/docker.sock, and on SELinux systems add security_opt: [label=disable]",
				i.endpoint,
			)
		}
		if i.endpointScheme == "unix" && isConnectionRefused(err) {
			return nil, fmt.Errorf(
				"query docker socket %q: connection refused; a socket exists at that path but no compatible API is listening. On Podman use the socket from the container host itself, often /run/user/1000/podman/podman.sock or /run/podman/podman.sock, and mount it to /var/run/docker.sock",
				i.endpoint,
			)
		}
		if isConnectionRefused(err) {
			return nil, fmt.Errorf(
				"query docker endpoint %q: connection refused; check that a Docker-compatible HTTP API is reachable at that address",
				i.endpoint,
			)
		}
		return nil, fmt.Errorf("query docker endpoint %q: %w", i.endpoint, err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf(
			"docker api returned %s; if you are using a Docker socket proxy, allow GET /containers/json (for Tecnativa/docker-socket-proxy set CONTAINERS=1)",
			response.Status,
		)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("docker api returned %s", response.Status)
	}

	var payload []container
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode docker containers response: %w", err)
	}
	return payload, nil
}

func resolveEndpoint(raw string) (endpointConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return endpointConfig{
			display: defaultSocket,
			scheme:  "unix",
			baseURL: defaultDockerAPIHost,
			socket:  defaultSocket,
		}, nil
	}

	if strings.HasPrefix(raw, "/") {
		return endpointConfig{
			display: raw,
			scheme:  "unix",
			baseURL: defaultDockerAPIHost,
			socket:  raw,
		}, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return endpointConfig{}, fmt.Errorf("parse docker endpoint %q: %w", raw, err)
	}

	switch parsed.Scheme {
	case "unix":
		if parsed.Host != "" {
			return endpointConfig{}, fmt.Errorf("docker endpoint %q is invalid; use unix:///absolute/path.sock for unix sockets", raw)
		}
		if parsed.Path == "" {
			return endpointConfig{}, fmt.Errorf("docker endpoint %q is invalid; unix endpoints need an absolute socket path", raw)
		}
		return endpointConfig{
			display: raw,
			scheme:  "unix",
			baseURL: defaultDockerAPIHost,
			socket:  parsed.Path,
		}, nil
	case "tcp":
		if parsed.Host == "" {
			return endpointConfig{}, fmt.Errorf("docker endpoint %q is invalid; tcp endpoints need host:port", raw)
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return endpointConfig{}, fmt.Errorf("docker endpoint %q is invalid; paths are not supported for tcp endpoints", raw)
		}
		return endpointConfig{
			display: raw,
			scheme:  "tcp",
			baseURL: "http://" + parsed.Host,
		}, nil
	case "http", "https":
		if parsed.Host == "" {
			return endpointConfig{}, fmt.Errorf("docker endpoint %q is invalid; %s endpoints need host:port", raw, parsed.Scheme)
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return endpointConfig{}, fmt.Errorf("docker endpoint %q is invalid; paths are not supported for %s endpoints", raw, parsed.Scheme)
		}
		return endpointConfig{
			display: raw,
			scheme:  parsed.Scheme,
			baseURL: strings.TrimRight(raw, "/"),
		}, nil
	default:
		return endpointConfig{}, fmt.Errorf(
			"docker endpoint %q has unsupported scheme %q; supported schemes are unix, tcp, http, https",
			raw,
			parsed.Scheme,
		)
	}
}

func isPermissionDenied(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, os.ErrPermission)
	}
	return errors.Is(err, os.ErrPermission)
}

func isConnectionRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.ECONNREFUSED)
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

func buildObservation(item container) model.Observation {
	containerName := firstContainerName(item)
	imageName, imageTag := parseImageReference(item.Image)
	artifactName := shortArtifactName(imageName)
	currentVersion, source, versionLabelKey := detectVersion(item.Labels, imageTag)
	projectName := item.Labels["com.docker.compose.project"]
	serviceName := item.Labels["com.docker.compose.service"]
	if serviceName == "" {
		serviceName = containerName
	}

	attributes := map[string]string{
		"container_id":   shortContainerID(item.ID),
		"container_name": containerName,
		"status":         item.Status,
	}
	if projectName != "" {
		attributes["project_name"] = projectName
	}
	if imageTag != "" {
		attributes["image_tag"] = imageTag
	}
	if versionLabelKey != "" {
		attributes["version_label_key"] = versionLabelKey
	}

	return model.Observation{
		ServiceName:          serviceName,
		ArtifactType:         "container_image",
		ArtifactName:         artifactName,
		ArtifactRef:          item.Image,
		CurrentVersion:       currentVersion,
		CurrentVersionSource: source,
		ObservedVia:          "docker_socket",
		Attributes:           attributes,
	}
}

func shouldExclude(item container, selfMatchers []string, selectors []string) bool {
	if matchesSelf(item, selfMatchers) {
		return true
	}

	for _, selector := range selectors {
		if matchesLabelSelector(item.Labels, selector) {
			return true
		}
	}
	return false
}

func matchesSelf(item container, selfMatchers []string) bool {
	if len(selfMatchers) == 0 {
		return false
	}

	containerName := normalizeSelfMatcher(firstContainerName(item))
	shortID := normalizeSelfMatcher(shortContainerID(item.ID))
	fullID := normalizeSelfMatcher(item.ID)

	for _, matcher := range selfMatchers {
		if matcher == "" {
			continue
		}
		if matcher == fullID || matcher == shortID || strings.HasPrefix(fullID, matcher) {
			return true
		}
		if matcher == containerName {
			return true
		}
	}

	return false
}

func detectSelfMatchers() []string {
	seen := make(map[string]struct{})

	add := func(value string) {
		value = normalizeSelfMatcher(value)
		if value == "" {
			return
		}

		seen[value] = struct{}{}
		if dot := strings.IndexByte(value, '.'); dot > 0 {
			seen[value[:dot]] = struct{}{}
		}
		if looksLikeContainerID(value) && len(value) > 12 {
			seen[value[:12]] = struct{}{}
		}
	}

	add(os.Getenv("HOSTNAME"))
	if hostname, err := os.Hostname(); err == nil {
		add(hostname)
	}
	add(detectSelfContainerID("/proc/self/cgroup"))
	add(detectSelfContainerID("/proc/self/mountinfo"))

	matchers := make([]string, 0, len(seen))
	for value := range seen {
		matchers = append(matchers, value)
	}
	sort.Strings(matchers)
	return matchers
}

func detectSelfContainerID(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		for _, pattern := range selfContainerIDPatterns {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) == 2 {
				return matches[1]
			}
		}
	}

	return ""
}

func normalizeSelfMatcher(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "/")
	return strings.ToLower(value)
}

func looksLikeContainerID(value string) bool {
	if len(value) < 12 {
		return false
	}

	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func matchesLabelSelector(labels map[string]string, selector string) bool {
	if strings.Contains(selector, "=") {
		parts := strings.SplitN(selector, "=", 2)
		return labels[parts[0]] == parts[1]
	}
	_, ok := labels[selector]
	return ok
}

func detectVersion(labels map[string]string, imageTag string) (string, string, string) {
	for _, key := range versionLabelKeys {
		if value := labels[key]; value != "" {
			return value, "container_label", key
		}
	}
	if imageTag != "" {
		return imageTag, "image_tag", ""
	}
	return "unknown", "unknown", ""
}

func parseImageReference(image string) (string, string) {
	if image == "" {
		return "", ""
	}
	if strings.Contains(image, "@") {
		parts := strings.SplitN(image, "@", 2)
		return parts[0], ""
	}

	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		return image[:lastColon], image[lastColon+1:]
	}
	return image, ""
}

func shortArtifactName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "/")
	if value == "" {
		return ""
	}

	if slash := strings.LastIndex(value, "/"); slash >= 0 && slash < len(value)-1 {
		return value[slash+1:]
	}

	return value
}

func firstContainerName(item container) string {
	if len(item.Names) == 0 {
		return shortContainerID(item.ID)
	}
	return strings.TrimPrefix(item.Names[0], "/")
}

func shortContainerID(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
