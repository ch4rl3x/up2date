package ospackage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"up2date/common/model"
)

const defaultManager = "dpkg"

var errPackageNotInstalled = errors.New("package is not installed")

type Config struct {
	Manager string   `json:"manager,omitempty"`
	Names   []string `json:"names,omitempty"`
}

type Collector struct {
	lookup versionLookup
	names  []string
}

type versionLookup interface {
	Version(ctx context.Context, packageName string) (string, error)
	Manager() string
	Source() string
}

type dpkgLookup struct{}
type brewLookup struct{}

type brewInfoPayload struct {
	Formulae []brewFormula `json:"formulae"`
}

type brewFormula struct {
	Name      string          `json:"name"`
	Installed []brewInstalled `json:"installed"`
}

type brewInstalled struct {
	Version string `json:"version"`
}

func New(cfg Config) (*Collector, error) {
	lookup, err := newVersionLookup(cfg.Manager)
	if err != nil {
		return nil, err
	}

	names := normalizeNames(cfg.Names)
	if len(names) == 0 {
		return nil, fmt.Errorf("package collector requires at least one package name")
	}

	return &Collector{
		lookup: lookup,
		names:  names,
	}, nil
}

func (c *Collector) Collect(ctx context.Context, node model.Node, jobName string) (model.Snapshot, error) {
	observations := make([]model.Observation, 0, len(c.names))
	for _, name := range c.names {
		version, installationState, err := c.collectVersion(ctx, name)
		if err != nil {
			return model.Snapshot{}, fmt.Errorf("collect package %q: %w", name, err)
		}
		observations = append(observations, buildObservation(name, version, installationState, c.lookup))
	}

	sort.Slice(observations, func(left, right int) bool {
		return observations[left].ServiceName < observations[right].ServiceName
	})

	return model.NewSnapshot(node, jobName, time.Now().UTC(), observations), nil
}

func (c *Collector) collectVersion(ctx context.Context, packageName string) (string, string, error) {
	version, err := c.lookup.Version(ctx, packageName)
	if err == nil {
		return version, "installed", nil
	}
	if errors.Is(err, errPackageNotInstalled) {
		return "", "not_installed", nil
	}
	return "", "", err
}

func buildObservation(packageName, version, installationState string, lookup versionLookup) model.Observation {
	return model.Observation{
		ServiceName:          packageName,
		ArtifactType:         "os_package",
		ArtifactName:         packageName,
		ArtifactRef:          lookup.Manager() + ":" + packageName,
		CurrentVersion:       version,
		CurrentVersionSource: lookup.Source(),
		ObservedVia:          "local_package_manager",
		Attributes: map[string]string{
			"installation_state": installationState,
			"package_manager":    lookup.Manager(),
			"package_name":       packageName,
		},
	}
}

func newVersionLookup(manager string) (versionLookup, error) {
	switch normalizeManager(manager) {
	case "dpkg":
		return dpkgLookup{}, nil
	case "brew", "homebrew":
		return brewLookup{}, nil
	default:
		return nil, fmt.Errorf("unsupported package manager %q", strings.TrimSpace(manager))
	}
}

func normalizeManager(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return defaultManager
	}
	return value
}

func normalizeNames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	names := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func (dpkgLookup) Version(ctx context.Context, packageName string) (string, error) {
	command := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Version}\n", packageName)
	output, err := command.CombinedOutput()
	if err != nil {
		if isPackageNotInstalled(output, err) {
			return "", errPackageNotInstalled
		}
		message := strings.TrimSpace(string(output))
		if message != "" {
			return "", fmt.Errorf("run dpkg-query: %w: %s", err, message)
		}
		return "", fmt.Errorf("run dpkg-query: %w", err)
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("dpkg-query returned an empty version")
	}
	return version, nil
}

func (dpkgLookup) Manager() string {
	return "dpkg"
}

func (dpkgLookup) Source() string {
	return "dpkg-query"
}

func (brewLookup) Version(ctx context.Context, packageName string) (string, error) {
	command := exec.CommandContext(ctx, "brew", "info", "--formula", "--json=v2", packageName)
	output, err := command.CombinedOutput()
	if err != nil {
		if isBrewPackageNotInstalled(output, err) {
			return "", errPackageNotInstalled
		}
		message := strings.TrimSpace(string(output))
		if message != "" {
			return "", fmt.Errorf("run brew list --versions: %w: %s", err, message)
		}
		return "", fmt.Errorf("run brew list --versions: %w", err)
	}

	version, installed, err := parseBrewInfoVersion(output, packageName)
	if err != nil {
		return "", err
	}
	if !installed {
		return "", errPackageNotInstalled
	}
	return version, nil
}

func (brewLookup) Manager() string {
	return "brew"
}

func (brewLookup) Source() string {
	return "brew info --json=v2"
}

func isPackageNotInstalled(output []byte, err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(string(output)))
	return strings.Contains(message, "is not installed") ||
		strings.Contains(message, "no packages found matching")
}

func isBrewPackageNotInstalled(output []byte, err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(string(output)))
	return strings.Contains(message, "no available formula") ||
		strings.Contains(message, "formulae found in taps")
}

func parseBrewInfoVersion(output []byte, packageName string) (string, bool, error) {
	var payload brewInfoPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return "", false, fmt.Errorf("decode brew info payload for %q: %w", packageName, err)
	}

	if len(payload.Formulae) != 1 {
		return "", false, fmt.Errorf("brew info returned %d formula entries for %q", len(payload.Formulae), packageName)
	}

	formula := payload.Formulae[0]
	if formula.Name != packageName {
		return "", false, fmt.Errorf("brew info returned package %q while %q was requested", formula.Name, packageName)
	}

	if len(formula.Installed) == 0 {
		return "", false, nil
	}

	version := strings.TrimSpace(formula.Installed[len(formula.Installed)-1].Version)
	if version == "" {
		return "", false, fmt.Errorf("brew info returned an empty installed version for %q", packageName)
	}

	return version, true, nil
}
