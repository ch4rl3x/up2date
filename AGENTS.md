# AGENTS

This file defines the intended direction for future coding work on `up2date`.

## North Star

`up2date` is not an auto-updater. It is an observability and decision-support tool for version drift across heterogeneous infrastructure.

The current MVP output is:

- what containers exist on a node
- which versions or tags they advertise
- when that node was last observed
- whether a newer registry tag appears to be available for a service

## Non-Negotiable Design Boundaries

1. Keep current-state detection separate from future latest-version resolution.
2. Keep version comparison logic explicit and configurable when it is introduced.
3. Do not assume SemVer everywhere.
4. Prefer agent or sidecar collection over central privileged access when both are possible.
5. Every observation must preserve provenance.
6. A generic system is a plugin system, not a giant if/else tree.
7. Prefer a small per-service MQTT contract over broad transport payloads.

## Preferred Domain Terms

- `collector`: finds the current deployed version or deployment artifact
- `resolver`: finds the latest relevant version upstream
- `comparator`: decides whether current and latest imply an update
- `observation`: one current-state fact with provenance
- `check result`: one comparison outcome for a tracked workload
- `node`: a reporting source, such as a host, VM, or sidecar agent
- `snapshot`: the internal normalized collection result for one node

## MVP Priority

Only optimize for these capabilities first:

- Docker Engine collection
- Docker Compose-aware metadata extraction
- MQTT per-service field publishing
- retained state that is easy to inspect in MQTT Explorer
- stable topic contracts for published per-service fields

Treat the following as later additions:

- backend subscriber and UI
- SSH collectors
- HTTP regex collectors
- Proxmox-native inventory
- package-manager integrations
- notifications
- automatic update workflows

## Architecture Guardrails

Before adding a new integration, document:

1. What input it requires.
2. What normalized output it emits.
3. How secrets are handled.
4. Which comparator strategy it expects.
5. What failure modes should be visible to the user.

Every collector should emit a normalized result shape. When resolver and comparison layers are added, they should do the same. Comparison should not depend on the original source format.

## Runtime Guidance

Current implementation:

- Go-based collector, resolver, orchestrator, and publisher modules

Fallback if the team strongly prefers Kotlin later:

- use Kotlin/JVM, not Kotlin Multiplatform, for the first delivery

If direction changes, keep [README.md](/Users/alex/Workspace/up2date/README.md) and this file aligned.

## Implementation Rule of Thumb

If there is a tension between "generic" and "shippable", choose the smallest abstraction that still keeps future plugin boundaries clean.
