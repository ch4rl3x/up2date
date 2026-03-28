package none

import (
	"context"

	"up2date/common/model"
)

type Resolver struct{}

func New() *Resolver {
	return &Resolver{}
}

func (r *Resolver) Resolve(_ context.Context, snapshot model.Snapshot) ([]model.CheckResult, error) {
	results := make([]model.CheckResult, 0, len(snapshot.Observations))
	for _, observation := range snapshot.Observations {
		check := model.NewCheckResult(snapshot, observation, "none")
		check.CheckStatus = "unknown"
		check.Reason = "current version tracked; latest version resolution is not configured"
		results = append(results, check)
	}
	return results, nil
}
