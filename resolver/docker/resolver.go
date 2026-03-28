package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"up2date/common/model"
)

const defaultTimeout = 20 * time.Second

const (
	registryKindDockerHub = "docker_hub"
	registryKindGHCR      = "ghcr"
)

var (
	versionRe   = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)([-._][A-Za-z0-9][A-Za-z0-9._-]*)?$`)
	authParamRe = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

type Resolver struct {
	client *registryClient
}

type registryClient struct {
	httpClient *http.Client
}

type registryReference struct {
	Kind       string
	Registry   string
	Repository string
	Display    string
	APIBaseURL string
}

type parsedVersion struct {
	Raw     string
	Numbers []int
	Suffix  string
}

type resolution struct {
	latestVersion    string
	latestVersionURL string
	status           string
	update           *bool
	reason           string
}

func New() *Resolver {
	return &Resolver{
		client: &registryClient{
			httpClient: &http.Client{Timeout: defaultTimeout},
		},
	}
}

func (r *Resolver) Resolve(ctx context.Context, snapshot model.Snapshot) ([]model.CheckResult, error) {
	results := make([]model.CheckResult, 0, len(snapshot.Observations))
	cache := map[string][]string{}

	for _, observation := range snapshot.Observations {
		check := model.NewCheckResult(snapshot, observation, "docker")
		reportedCurrentVersion := selectReportedCurrentVersion(observation)
		if reportedCurrentVersion != "" {
			check.CurrentVersion = reportedCurrentVersion
		}
		comparisonVersion := selectComparisonVersion(observation)

		ref, err := parseRegistryReference(selectArtifactReference(observation))
		if err != nil {
			check.CheckStatus = "unsupported"
			check.Reason = err.Error()
			results = append(results, check)
			continue
		}

		if parseVersion(comparisonVersion) == nil {
			check.CheckStatus = "unsupported"
			check.Reason = "current version is not parseable as a numeric release tag"
			results = append(results, check)
			continue
		}

		cacheKey := ref.Registry + "/" + ref.Repository
		tags, ok := cache[cacheKey]
		if !ok {
			tags, err = r.client.listTags(ctx, ref)
			if err != nil {
				check.CheckStatus = "error"
				check.Reason = err.Error()
				results = append(results, check)
				continue
			}
			cache[cacheKey] = tags
		}

		resolution := determineResolution(observation, ref, tags)
		check.CheckStatus = resolution.status
		check.UpdateAvailable = resolution.update
		check.Reason = resolution.reason
		if resolution.latestVersion != "" {
			check.LatestVersion = resolution.latestVersion
		}
		if resolution.latestVersionURL != "" {
			check.LatestVersionURL = resolution.latestVersionURL
		}

		results = append(results, check)
	}

	return results, nil
}

func determineResolution(observation model.Observation, ref registryReference, tags []string) resolution {
	currentVersion := selectComparisonVersion(observation)
	parsedCurrent := parseVersion(currentVersion)
	if parsedCurrent == nil {
		return resolution{
			status: "unsupported",
			reason: "current version is not parseable as a numeric release tag",
		}
	}

	candidates := make([]parsedVersion, 0, len(tags))
	for _, tag := range tags {
		if parsed := parseVersion(tag); parsed != nil {
			candidates = append(candidates, *parsed)
		}
	}

	if len(candidates) == 0 {
		return resolution{
			status: "unknown",
			reason: "registry did not return any parseable release tags",
		}
	}

	filtered := make([]parsedVersion, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.Numbers) == len(parsedCurrent.Numbers) &&
			candidate.Suffix == parsedCurrent.Suffix {
			filtered = append(filtered, candidate)
		}
	}

	if len(filtered) == 0 {
		return resolution{
			status: "unknown",
			reason: "no registry tags matched the current version scheme with high confidence",
		}
	}

	sort.Slice(filtered, func(left, right int) bool {
		return compareNumbers(filtered[left].Numbers, filtered[right].Numbers) < 0
	})
	latest := filtered[len(filtered)-1]
	updateAvailable := compareNumbers(parsedCurrent.Numbers, latest.Numbers) < 0
	status := "current"
	if updateAvailable {
		status = "outdated"
	}

	return resolution{
		latestVersion:    latest.Raw,
		latestVersionURL: buildLatestVersionURL(ref, latest.Raw),
		status:           status,
		update:           model.Bool(updateAvailable),
	}
}

func buildLatestVersionURL(ref registryReference, tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}

	switch ref.Kind {
	case registryKindDockerHub:
		return buildDockerHubLatestVersionURL(ref.Repository, tag)
	default:
		return ""
	}
}

func buildDockerHubLatestVersionURL(repository, tag string) string {
	repository = strings.TrimSpace(repository)
	tag = strings.TrimSpace(tag)
	if repository == "" || tag == "" {
		return ""
	}
	if officialImageName, ok := strings.CutPrefix(repository, "library/"); ok && officialImageName != "" {
		return "https://hub.docker.com/_/" + officialImageName + "/tags?name=" + url.QueryEscape(tag)
	}
	return "https://hub.docker.com/r/" + repository + "/tags?name=" + url.QueryEscape(tag)
}

func selectReportedCurrentVersion(observation model.Observation) string {
	if parseVersion(observation.CurrentVersion) != nil {
		return observation.CurrentVersion
	}
	imageTag := observation.Attributes["image_tag"]
	if parseVersion(imageTag) != nil {
		return imageTag
	}
	if observation.CurrentVersion != "" {
		return observation.CurrentVersion
	}
	return imageTag
}

func selectComparisonVersion(observation model.Observation) string {
	imageTag := strings.TrimSpace(observation.Attributes["image_tag"])
	if imageTag != "" {
		return imageTag
	}
	return strings.TrimSpace(observation.CurrentVersion)
}

func selectArtifactReference(observation model.Observation) string {
	if strings.TrimSpace(observation.ArtifactRef) != "" {
		return observation.ArtifactRef
	}
	return observation.ArtifactName
}

func parseRegistryReference(imageRef string) (registryReference, error) {
	imageRef = normalizeImageReference(imageRef)
	if imageRef == "" {
		return registryReference{}, fmt.Errorf("observation is missing artifact reference")
	}

	host, repository := splitRegistryHost(imageRef)
	kind, registryHost, apiBaseURL, err := resolveRegistryHost(host)
	if err != nil {
		return registryReference{}, err
	}
	if repository == "" {
		return registryReference{}, fmt.Errorf("image reference %q is missing a repository", imageRef)
	}
	if kind == registryKindDockerHub && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return registryReference{
		Kind:       kind,
		Registry:   registryHost,
		Repository: repository,
		Display:    registryHost + "/" + repository,
		APIBaseURL: apiBaseURL,
	}, nil
}

func splitRegistryHost(imageRef string) (string, string) {
	parts := strings.Split(imageRef, "/")
	if len(parts) > 1 && isRegistryHost(parts[0]) {
		return strings.ToLower(parts[0]), strings.Join(parts[1:], "/")
	}
	return "docker.io", imageRef
}

func isRegistryHost(value string) bool {
	return strings.Contains(value, ".") || strings.Contains(value, ":") || value == "localhost"
}

func resolveRegistryHost(host string) (string, string, string, error) {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "docker.io", "index.docker.io", "registry-1.docker.io":
		return registryKindDockerHub, "docker.io", "https://registry-1.docker.io", nil
	case "ghcr.io":
		return registryKindGHCR, "ghcr.io", "https://ghcr.io", nil
	default:
		return "", "", "", fmt.Errorf("docker resolver does not support registry %q", host)
	}
}

func normalizeImageReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if at := strings.IndexByte(value, '@'); at >= 0 {
		value = value[:at]
	}

	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		return value[:lastColon]
	}

	return value
}

func parseVersion(value string) *parsedVersion {
	match := versionRe.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return nil
	}

	rawNumbers := strings.Split(match[1], ".")
	numbers := make([]int, 0, len(rawNumbers))
	for _, raw := range rawNumbers {
		number, err := strconv.Atoi(raw)
		if err != nil {
			return nil
		}
		numbers = append(numbers, number)
	}

	return &parsedVersion{
		Raw:     strings.TrimSpace(value),
		Numbers: numbers,
		Suffix:  match[2],
	}
}

func compareNumbers(left, right []int) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func (c *registryClient) listTags(ctx context.Context, ref registryReference) ([]string, error) {
	token := ""
	nextURL := ref.APIBaseURL + "/v2/" + ref.Repository + "/tags/list?n=1000"
	tagSet := map[string]struct{}{}

	for nextURL != "" {
		response, err := c.request(ctx, nextURL, token, ref.Registry)
		if err != nil {
			return nil, err
		}

		if response.StatusCode == http.StatusUnauthorized {
			authHeader := response.Header.Get("WWW-Authenticate")
			response.Body.Close()
			if authHeader == "" {
				return nil, fmt.Errorf("%s requested auth without a challenge header", ref.Registry)
			}
			token, err = c.fetchToken(ctx, ref, authHeader)
			if err != nil {
				return nil, err
			}
			continue
		}

		if response.StatusCode >= http.StatusBadRequest {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			return nil, fmt.Errorf("%s returned %s: %s", ref.Registry, response.Status, strings.TrimSpace(string(body)))
		}

		var payload struct {
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return nil, fmt.Errorf("decode %s tags response: %w", ref.Registry, err)
		}
		response.Body.Close()

		for _, tag := range payload.Tags {
			tagSet[tag] = struct{}{}
		}

		nextURL = linkNextURL(ref.APIBaseURL, response.Header.Get("Link"))
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

func (c *registryClient) fetchToken(ctx context.Context, ref registryReference, header string) (string, error) {
	params := parseBearerChallenge(header)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("%s auth challenge is missing a realm", ref.Registry)
	}

	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("parse %s token url: %w", ref.Registry, err)
	}

	query := tokenURL.Query()
	if service := params["service"]; service != "" {
		query.Set("service", service)
	}
	scope := params["scope"]
	if scope == "" {
		scope = "repository:" + ref.Repository + ":pull"
	}
	query.Set("scope", scope)
	tokenURL.RawQuery = query.Encode()

	response, err := c.request(ctx, tokenURL.String(), "", ref.Registry)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("%s token endpoint returned %s: %s", ref.Registry, response.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode %s token response: %w", ref.Registry, err)
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("%s token endpoint did not return a bearer token", ref.Registry)
}

func (c *registryClient) request(ctx context.Context, rawURL, token, registry string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", registry, err)
	}
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request %s %q: %w", registry, rawURL, err)
	}
	return response, nil
}

func parseBearerChallenge(header string) map[string]string {
	match := strings.TrimSpace(header)
	if len(match) >= 7 && strings.EqualFold(match[:7], "Bearer ") {
		match = match[7:]
	}

	params := map[string]string{}
	for _, part := range authParamRe.FindAllStringSubmatch(match, -1) {
		params[part[1]] = part[2]
	}
	return params
}

func linkNextURL(baseURL, linkHeader string) string {
	linkHeader = strings.TrimSpace(linkHeader)
	if linkHeader == "" || !strings.Contains(linkHeader, ";") {
		return ""
	}

	parts := strings.SplitN(linkHeader, ";", 2)
	if !strings.Contains(parts[1], `rel="next"`) {
		return ""
	}

	target := strings.TrimSpace(parts[0])
	target = strings.TrimPrefix(target, "<")
	target = strings.TrimSuffix(target, ">")

	targetURL, err := url.Parse(target)
	if err != nil {
		return ""
	}
	if targetURL.IsAbs() {
		return targetURL.String()
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return base.ResolveReference(targetURL).String()
}
