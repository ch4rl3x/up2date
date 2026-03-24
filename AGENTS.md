# AGENTS

This file defines the intended direction for future contributors and coding agents working on `up2date`.

## North Star

`up2date` is not an auto-updater. It is an observability and decision-support tool for version drift across heterogeneous infrastructure.

The long-term output is trustworthy update intelligence:

- what is deployed
- how that version was detected
- what upstream version is relevant
- whether the instance is behind
- why the system believes that result

The current MVP output is narrower:

- what containers exist on a node
- which versions or tags they advertise
- whether they are running right now
- when that node was last observed
- whether a newer registry tag appears to be available for a service

## Non-Negotiable Design Boundaries

1. Keep current-state detection separate from future latest-version resolution.
2. Keep version comparison logic explicit and configurable when it is introduced.
3. Do not assume SemVer everywhere.
4. Prefer agent or sidecar collection over central privileged access when both are possible.
5. Every observation must preserve provenance.
6. A generic system is a plugin system, not a giant if/else tree.
7. The MQTT snapshot contract is the MVP source of truth.

## Preferred Domain Terms

- `collector`: finds the current running version or deployment artifact
- `resolver`: finds the latest relevant version upstream
- `comparator`: decides whether current and latest imply an update
- `observation`: one current-state fact with provenance
- `check result`: one comparison outcome for a tracked workload
- `node`: a reporting source, such as a host, VM, or sidecar agent
- `snapshot`: the full current-state MQTT payload for one node

## MVP Priority

Only optimize for these capabilities first:

- Docker Engine collection
- Docker Compose-aware metadata extraction
- MQTT snapshot publishing
- MQTT per-service check publishing
- retained state that is easy to inspect in MQTT Explorer
- stable payload schemas for both snapshots and checks

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

## Language Guidance

Current long-term recommendation:

- core runtime in Go
- web UI in TypeScript + React

Current prototype implementation:

- Python standard library only under [agent](/Users/alex/Workspace/up2date/agent)
- Python standard library only under [resolver](/Users/alex/Workspace/up2date/resolver)

Fallback if the team strongly prefers Kotlin later:

- use Kotlin/JVM, not Kotlin Multiplatform, for the first delivery

If this recommendation changes, update the ADR before large implementation work continues.

## Documentation Discipline

When changing direction, keep these files in sync:

- [README.md](/Users/alex/Workspace/up2date/README.md)
- [docs/vision.md](/Users/alex/Workspace/up2date/docs/vision.md)
- [docs/architecture.md](/Users/alex/Workspace/up2date/docs/architecture.md)
- [docs/mqtt.md](/Users/alex/Workspace/up2date/docs/mqtt.md)
- [docs/roadmap.md](/Users/alex/Workspace/up2date/docs/roadmap.md)
- [docs/adr/0001-language-and-runtime.md](/Users/alex/Workspace/up2date/docs/adr/0001-language-and-runtime.md)
- [docs/adr/0002-mqtt-first-mvp.md](/Users/alex/Workspace/up2date/docs/adr/0002-mqtt-first-mvp.md)

## Implementation Rule of Thumb

If there is a tension between "generic" and "shippable", choose the smallest abstraction that still keeps future plugin boundaries clean.
