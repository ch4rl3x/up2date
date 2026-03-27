package dockerhub

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

type hubReference struct {
	Repository string
	Display    string
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
		check := model.NewCheckResult(snapshot, observation, "docker_hub")
		currentVersion := selectCurrentVersion(observation)
		if currentVersion != "" {
			check.CurrentVersion = currentVersion
		}

		ref, err := parseDockerHubReference(selectArtifactReference(observation))
		if err != nil {
			check.CheckStatus = "unsupported"
			check.Reason = err.Error()
			results = append(results, check)
			continue
		}

		if parseVersion(currentVersion) == nil {
			check.CheckStatus = "unsupported"
			check.Reason = "current version is not parseable as a numeric release tag"
			results = append(results, check)
			continue
		}

		tags, ok := cache[ref.Repository]
		if !ok {
			tags, err = r.client.listTags(ctx, ref)
			if err != nil {
				check.CheckStatus = "error"
				check.Reason = err.Error()
				results = append(results, check)
				continue
			}
			cache[ref.Repository] = tags
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

func determineResolution(observation model.Observation, ref hubReference, tags []string) resolution {
	currentVersion := selectCurrentVersion(observation)
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
			reason: "docker hub did not return any parseable release tags",
		}
	}

	imageTag := observation.Attributes["image_tag"]
	parsedImageTag := parseVersion(imageTag)
	filtered := make([]parsedVersion, 0, len(candidates))
	if parsedImageTag != nil && len(parsedImageTag.Numbers) == 1 {
		for _, candidate := range candidates {
			if len(candidate.Numbers) == len(parsedCurrent.Numbers) &&
				candidate.Suffix == parsedCurrent.Suffix &&
				candidate.Numbers[0] == parsedImageTag.Numbers[0] {
				filtered = append(filtered, candidate)
			}
		}
	} else {
		for _, candidate := range candidates {
			if len(candidate.Numbers) == len(parsedCurrent.Numbers) &&
				candidate.Suffix == parsedCurrent.Suffix {
				filtered = append(filtered, candidate)
			}
		}
	}

	if len(filtered) == 0 {
		return resolution{
			status: "unknown",
			reason: "no docker hub tags matched the current version scheme with high confidence",
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
		latestVersionURL: buildLatestVersionURL(ref.Repository, latest.Raw),
		status:           status,
		update:           model.Bool(updateAvailable),
	}
}

func buildLatestVersionURL(repository, tag string) string {
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

func selectCurrentVersion(observation model.Observation) string {
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

func selectArtifactReference(observation model.Observation) string {
	if strings.TrimSpace(observation.ArtifactRef) != "" {
		return observation.ArtifactRef
	}
	return observation.ArtifactName
}

func parseDockerHubReference(imageRef string) (hubReference, error) {
	imageRef = normalizeImageReference(imageRef)
	if imageRef == "" {
		return hubReference{}, fmt.Errorf("observation is missing artifact reference")
	}

	parts := strings.Split(imageRef, "/")
	if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		switch parts[0] {
		case "docker.io", "index.docker.io", "registry-1.docker.io":
			repository := strings.Join(parts[1:], "/")
			if repository == "" {
				return hubReference{}, fmt.Errorf("docker hub image reference %q is missing a repository", imageRef)
			}
			return hubReference{
				Repository: repository,
				Display:    "docker.io/" + repository,
			}, nil
		default:
			return hubReference{}, fmt.Errorf("docker_hub resolver only supports Docker Hub images")
		}
	}

	repository := imageRef
	if !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return hubReference{
		Repository: repository,
		Display:    "docker.io/" + repository,
	}, nil
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

func (c *registryClient) listTags(ctx context.Context, ref hubReference) ([]string, error) {
	token := ""
	nextURL := fmt.Sprintf("https://registry-1.docker.io/v2/%s/tags/list?n=1000", ref.Repository)
	tagSet := map[string]struct{}{}

	for nextURL != "" {
		response, err := c.request(ctx, nextURL, token)
		if err != nil {
			return nil, err
		}

		if response.StatusCode == http.StatusUnauthorized {
			authHeader := response.Header.Get("WWW-Authenticate")
			response.Body.Close()
			if authHeader == "" {
				return nil, fmt.Errorf("docker hub requested auth without a challenge header")
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
			return nil, fmt.Errorf("docker hub returned %s: %s", response.Status, strings.TrimSpace(string(body)))
		}

		var payload struct {
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return nil, fmt.Errorf("decode docker hub tags response: %w", err)
		}
		response.Body.Close()

		for _, tag := range payload.Tags {
			tagSet[tag] = struct{}{}
		}

		nextURL = linkNextURL(response.Header.Get("Link"))
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

func (c *registryClient) fetchToken(ctx context.Context, ref hubReference, header string) (string, error) {
	params := parseBearerChallenge(header)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("docker hub auth challenge is missing a realm")
	}

	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("parse docker hub token url: %w", err)
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

	response, err := c.request(ctx, tokenURL.String(), "")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("docker hub token endpoint returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode docker hub token response: %w", err)
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("docker hub token endpoint did not return a bearer token")
}

func (c *registryClient) request(ctx context.Context, rawURL, token string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build docker hub request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request docker hub %q: %w", rawURL, err)
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

func linkNextURL(linkHeader string) string {
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
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	if strings.HasPrefix(target, "/") {
		return "https://registry-1.docker.io" + target
	}
	return "https://registry-1.docker.io/" + target
}
