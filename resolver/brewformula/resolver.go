package brewformula

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"up2date/common/model"
)

type Resolver struct {
	client client
}

type client interface {
	info(ctx context.Context, formulaName string) (infoFormula, error)
	outdated(ctx context.Context, formulaName string) (outdatedFormula, bool, error)
}

type brewClient struct{}

type infoPayload struct {
	Formulae []infoFormula `json:"formulae"`
}

type infoFormula struct {
	Name      string          `json:"name"`
	Installed []infoInstalled `json:"installed"`
	Outdated  bool            `json:"outdated"`
}

type infoInstalled struct {
	Version string `json:"version"`
}

type outdatedPayload struct {
	Formulae []outdatedFormula `json:"formulae"`
}

type outdatedFormula struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
}

func New() *Resolver {
	return &Resolver{
		client: brewClient{},
	}
}

func (r *Resolver) Resolve(ctx context.Context, snapshot model.Snapshot) ([]model.CheckResult, error) {
	results := make([]model.CheckResult, 0, len(snapshot.Observations))
	infoCache := make(map[string]infoFormula)
	outdatedCache := make(map[string]outdatedFormula)

	for _, observation := range snapshot.Observations {
		check := model.NewCheckResult(snapshot, observation, "brew_formula")
		formulaName, ok := selectFormulaName(observation)
		if !ok {
			check.CheckStatus = "unsupported"
			check.Reason = "brew_formula resolver only supports brew package observations"
			results = append(results, check)
			continue
		}

		info, ok := infoCache[formulaName]
		if !ok {
			var err error
			info, err = r.client.info(ctx, formulaName)
			if err != nil {
				check.CheckStatus = "error"
				check.Reason = err.Error()
				results = append(results, check)
				continue
			}
			infoCache[formulaName] = info
		}

		check.LatestVersionURL = buildFormulaURL(formulaName)

		if len(info.Installed) == 0 || strings.TrimSpace(check.CurrentVersion) == "" {
			check.CheckStatus = "unknown"
			check.Reason = "formula is not installed"
			results = append(results, check)
			continue
		}

		if !info.Outdated {
			check.LatestVersion = check.CurrentVersion
			check.CheckStatus = "current"
			check.UpdateAvailable = model.Bool(false)
			results = append(results, check)
			continue
		}

		outdated, ok := outdatedCache[formulaName]
		if !ok {
			var found bool
			var err error
			outdated, found, err = r.client.outdated(ctx, formulaName)
			if err != nil {
				check.CheckStatus = "error"
				check.Reason = err.Error()
				results = append(results, check)
				continue
			}
			if found {
				outdatedCache[formulaName] = outdated
			}
		}

		latestVersion := strings.TrimSpace(outdated.CurrentVersion)
		if latestVersion == "" {
			check.CheckStatus = "unknown"
			check.Reason = "formula is marked outdated but brew did not return a latest version"
			results = append(results, check)
			continue
		}

		check.LatestVersion = latestVersion
		check.CheckStatus = "outdated"
		check.UpdateAvailable = model.Bool(true)
		results = append(results, check)
	}

	return results, nil
}

func selectFormulaName(observation model.Observation) (string, bool) {
	if strings.TrimSpace(observation.Attributes["package_manager"]) != "brew" {
		return "", false
	}

	if name := strings.TrimSpace(observation.Attributes["package_name"]); name != "" {
		return name, true
	}

	ref := strings.TrimSpace(observation.ArtifactRef)
	if name, ok := strings.CutPrefix(ref, "brew:"); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name), true
	}

	if name := strings.TrimSpace(observation.ArtifactName); name != "" {
		return name, true
	}

	return "", false
}

func buildFormulaURL(formulaName string) string {
	formulaName = strings.TrimSpace(formulaName)
	if formulaName == "" {
		return ""
	}
	return "https://formulae.brew.sh/formula/" + url.PathEscape(formulaName)
}

func (brewClient) info(ctx context.Context, formulaName string) (infoFormula, error) {
	output, err := runBrewJSON(ctx, "info", "--formula", "--json=v2", formulaName)
	if err != nil {
		return infoFormula{}, err
	}

	var payload infoPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return infoFormula{}, fmt.Errorf("decode brew info payload for %q: %w", formulaName, err)
	}
	if len(payload.Formulae) != 1 {
		return infoFormula{}, fmt.Errorf("brew info returned %d formula entries for %q", len(payload.Formulae), formulaName)
	}
	return payload.Formulae[0], nil
}

func (brewClient) outdated(ctx context.Context, formulaName string) (outdatedFormula, bool, error) {
	output, err := runBrewJSONAllowExitOne(ctx, "outdated", "--json=v2", formulaName)
	if err != nil {
		return outdatedFormula{}, false, err
	}

	var payload outdatedPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return outdatedFormula{}, false, fmt.Errorf("decode brew outdated payload for %q: %w", formulaName, err)
	}
	if len(payload.Formulae) == 0 {
		return outdatedFormula{}, false, nil
	}
	if len(payload.Formulae) != 1 {
		return outdatedFormula{}, false, fmt.Errorf("brew outdated returned %d formula entries for %q", len(payload.Formulae), formulaName)
	}
	return payload.Formulae[0], true, nil
}

func runBrewJSON(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "brew", args...)
	command.Env = append(command.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return nil, fmt.Errorf("run brew %s: %w: %s", strings.Join(args, " "), err, message)
		}
		return nil, fmt.Errorf("run brew %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func runBrewJSONAllowExitOne(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "brew", args...)
	command.Env = append(command.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	if !strings.HasPrefix(strings.TrimSpace(string(output)), "{") || !strings.Contains(string(output), "\"formulae\"") {
		if message := strings.TrimSpace(string(output)); message != "" {
			return nil, fmt.Errorf("run brew %s: %w: %s", strings.Join(args, " "), err, message)
		}
		return nil, fmt.Errorf("run brew %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}
