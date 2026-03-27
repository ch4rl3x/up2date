package model

import (
	"sort"
	"time"
)

type Node struct {
	ID string
}

type Snapshot struct {
	SchemaVersion int           `json:"schema_version"`
	Kind          string        `json:"kind"`
	NodeID        string        `json:"node_id"`
	JobName       string        `json:"job_name"`
	ObservedAt    string        `json:"observed_at"`
	Observations  []Observation `json:"observations"`
}

type Observation struct {
	ServiceName          string            `json:"service_name"`
	ArtifactType         string            `json:"artifact_type,omitempty"`
	ArtifactName         string            `json:"artifact_name,omitempty"`
	ArtifactRef          string            `json:"artifact_ref,omitempty"`
	CurrentVersion       string            `json:"current_version,omitempty"`
	CurrentVersionSource string            `json:"current_version_source,omitempty"`
	ObservedVia          string            `json:"observed_via"`
	Attributes           map[string]string `json:"attributes,omitempty"`
}

type CheckResult struct {
	SchemaVersion        int    `json:"schema_version"`
	Kind                 string `json:"kind"`
	NodeID               string `json:"node_id"`
	JobName              string `json:"job_name"`
	ServiceName          string `json:"service_name"`
	ObservedAt           string `json:"observed_at"`
	ArtifactType         string `json:"artifact_type,omitempty"`
	ArtifactName         string `json:"artifact_name,omitempty"`
	ArtifactRef          string `json:"artifact_ref,omitempty"`
	CurrentVersion       string `json:"current_version,omitempty"`
	CurrentVersionSource string `json:"current_version_source,omitempty"`
	ObservedVia          string `json:"observed_via,omitempty"`
	LatestVersion        string `json:"latest_version,omitempty"`
	LatestVersionURL     string `json:"latest_version_url,omitempty"`
	CheckStatus          string `json:"check_status"`
	UpdateAvailable      *bool  `json:"update_available"`
	Reason               string `json:"reason,omitempty"`
	Resolver             string `json:"resolver"`
}

func NewSnapshot(node Node, jobName string, observedAt time.Time, observations []Observation) Snapshot {
	sorted := append([]Observation(nil), observations...)
	sort.Slice(sorted, func(left, right int) bool {
		a := sorted[left]
		b := sorted[right]
		if a.ServiceName != b.ServiceName {
			return a.ServiceName < b.ServiceName
		}
		if a.ArtifactName != b.ArtifactName {
			return a.ArtifactName < b.ArtifactName
		}
		return a.ArtifactRef < b.ArtifactRef
	})

	return Snapshot{
		SchemaVersion: 1,
		Kind:          "node_snapshot",
		NodeID:        node.ID,
		JobName:       jobName,
		ObservedAt:    FormatTime(observedAt),
		Observations:  sorted,
	}
}

func NewCheckResult(snapshot Snapshot, observation Observation, resolverName string) CheckResult {
	return CheckResult{
		SchemaVersion:        1,
		Kind:                 "check_result",
		NodeID:               snapshot.NodeID,
		JobName:              snapshot.JobName,
		ServiceName:          observation.ServiceName,
		ObservedAt:           snapshot.ObservedAt,
		ArtifactType:         observation.ArtifactType,
		ArtifactName:         observation.ArtifactName,
		ArtifactRef:          observation.ArtifactRef,
		CurrentVersion:       observation.CurrentVersion,
		CurrentVersionSource: observation.CurrentVersionSource,
		ObservedVia:          observation.ObservedVia,
		CheckStatus:          "unknown",
		Resolver:             resolverName,
	}
}

func FormatTime(value time.Time) string {
	return value.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func Bool(value bool) *bool {
	return &value
}
