# AGENTS

This file defines the intended direction for future coding work on `up2date`.

## What `up2date` Is

`up2date` is a version-drift observability tool.

It is not:

- an auto-updater
- a general-purpose inventory database
- a remote execution framework
- a giant "monitor everything" platform

Its job is to answer a narrow but useful question:

"What is deployed here now, what does upstream say is newer, and how confident are we that this workload is behind?"

## Current System Shape

The current implementation is a single Go application with a simple pipeline:

1. a `collector` gathers current facts for one node and produces a normalized `snapshot`
2. a `resolver` enriches those observations with upstream version information and emits `check results`
3. a `publisher` publishes those results to a transport, currently MQTT
4. the `orchestrator` wires the modules together and runs them on an interval

Current concrete modules:

- `collector/docker`
- `collector/ospackage`
- `resolver/brewformula`
- `resolver/docker`
- `resolver/none`
- `publisher/mqtt`
- `common/model`
- `orchestrator`

That shape is intentional. New integrations should fit into that flow instead of bypassing it.

## Product Direction

The current MVP is intentionally narrow:

- observe Docker workloads on a node
- observe selected locally installed OS packages on a node
- detect a current version or tag from the deployed artifact
- resolve a newer relevant version from Docker registries such as Docker Hub or GHCR or, for Homebrew formulas, from Homebrew metadata
- publish a small retained per-service MQTT contract
- make the state easy to inspect in MQTT Explorer

The broader long-term direction is heterogeneous infrastructure version drift, not "Docker tooling forever". Future collectors may target other sources, but they must still map into the same conceptual pipeline.

## Core Domain Terms

Use these terms consistently:

- `node`: one reporting source, such as a host, VM, sidecar, or agent process
- `collector`: obtains current-state facts from one source
- `snapshot`: the normalized result of one collection run for one node
- `observation`: one normalized current-state fact within a snapshot
- `resolver`: looks up upstream or registry information for an observation
- `check result`: one enriched comparison outcome for one workload
- `artifact_name`: a collector-owned human-friendly name for the deployed artifact
- `artifact_ref`: the canonical upstream reference a resolver can use
- `check_status`: the resolver outcome such as `current`, `outdated`, `unsupported`, `error`, or `unknown`

Avoid inventing new overlapping terms unless there is a strong reason.

## Architectural Boundaries

These boundaries are non-negotiable:

1. Collectors own current-state facts.
   That includes `service_name`, `artifact_name`, `artifact_ref`, `current_version`, `current_version_source`, `observed_via`, and source-specific raw attributes.
2. Resolvers own upstream lookup and update judgment.
   They must not invent collector facts that should have been gathered at collection time.
3. Publishers only publish.
   They should not derive artifact identity, parse version strings, normalize Docker references, or make domain decisions.
4. Orchestrators only orchestrate.
   Scheduling, wiring, and lifecycle belong there. Source-specific logic does not.
5. Shared models must stay source-neutral where possible.
   When source-specific detail is unavoidable, keep it in `Attributes` or in the integration package, not as transport-driven top-level fields.

## Normalized Model Intent

The shared model in `common/model` is the contract between layers.

That means:

- a collector should be able to change without requiring publisher changes
- a resolver should consume normalized fields, especially `artifact_ref`, not source-specific transport quirks
- a publisher should be able to publish any `CheckResult` without knowing where it came from

For the current model, these meanings matter:

- `artifact_name` is the short, display-friendly identity chosen by the collector
- `artifact_ref` is the resolver-facing lookup reference
- `current_version` is the best current version signal the collector could determine
- `current_version_source` explains where that version came from
- `observed_via` preserves provenance
- `check_status` is the outcome of the resolver stage, not a runtime state field

Do not overload `check_status` with source lifecycle state.
Do not make `artifact_name` double as a registry lookup key.

## MQTT Contract Intent

The MQTT output is intentionally small and boring.

Current contract shape:

- `up2date/<node_id>/<service_name>/artifact_name`
- `up2date/<node_id>/<service_name>/current_version`
- `up2date/<node_id>/<service_name>/latest_version`
- `up2date/<node_id>/<service_name>/latest_version_url`
- `up2date/<node_id>/<service_name>/observed_at`
- `up2date/<node_id>/<service_name>/check_status`

Guidance for MQTT changes:

- prefer a few stable fields over broad payloads
- do not add collector-specific fields lightly
- do not leak raw source formats into MQTT
- if a field is only useful for one integration family, think twice before making it part of the global contract

If the contract changes, update [README.md](/Users/alex/Workspace/up2date/README.md) and this file together.

## How To Add A New Collector

A new collector belongs in `collector/<type>`.

Before adding one, define:

1. what input it reads
2. what one `Observation` means for that source
3. how it derives `artifact_name`
4. how it derives `artifact_ref`
5. how it derives `current_version`
6. which raw source details must remain in `Attributes`
7. how authentication or secrets are handled
8. which failure modes should be visible to the user

Collector rules:

- emit normalized observations, not transport-shaped data
- keep source-specific parsing inside the collector package
- prefer agent/sidecar or local access over central privileged scraping when both are viable
- never make a new collector depend on MQTT field names or publisher behavior

## How To Add A New Resolver

A new resolver belongs in `resolver/<type>`.

Resolver rules:

- resolve from normalized observation fields, especially `artifact_ref`
- produce `CheckResult` values with consistent semantics
- keep upstream API quirks inside the resolver package
- make unsupported cases explicit via `check_status` and `reason`
- do not silently assume SemVer for every source

If a resolver only works for a subset of artifacts, that limitation should be explicit in code and in documentation.

## Comparator Guidance

Today, comparison logic is still embedded inside the Docker resolver. That is acceptable for the current MVP.

When comparison rules start to diverge, extract a dedicated comparator layer such as:

- `comparator/semver`
- `comparator/docker_tag_family`
- `comparator/calendar_version`
- `comparator/custom`

Do that only when there is real duplication or multiple resolver families needing the same comparison strategy.

Do not prematurely create an abstraction that is broader than the current use cases.

## Package And Wiring Guidance

Prefer this structure for future growth:

- `collector/<type>`
- `resolver/<type>`
- `publisher/<type>`
- `comparator/<type>` when justified
- `common/model`
- `orchestrator`

The current switch-based wiring in `orchestrator/build.go` is acceptable for now.
If integrations grow, move toward a small registry or factory pattern.
Do not replace clean module boundaries with a giant if/else tree spread across the codebase.

## Current Priorities

Optimize for these first:

- Docker Engine collection
- package-manager collection for `dpkg` and Homebrew
- Compose-aware metadata extraction
- stable MQTT per-service fields
- retained MQTT state that is easy to inspect
- predictable collector and resolver boundaries

Treat these as later additions:

- backend subscriber and UI
- SSH collectors
- HTTP regex collectors
- Proxmox-native inventory
- additional package-manager integrations beyond `dpkg` and Homebrew
- notifications
- automated update workflows

## Implementation Guidance

- Keep the implementation in Go unless there is an explicit project-level decision to change languages.
- Prefer clear data flow over clever abstractions.
- Preserve provenance whenever a fact is transformed.
- Favor the smallest abstraction that keeps future plugin boundaries clean.
- If a change makes README and AGENTS diverge, fix both in the same change.

## Rule Of Thumb

When there is tension between "generic" and "shippable", choose the smallest design that:

- ships the current MVP cleanly
- keeps collector, resolver, and publisher responsibilities separate
- does not block future integration-specific packages

That is the bar. Not theoretical perfection.
