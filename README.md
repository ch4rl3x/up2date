# up2date

`up2date` is a version observability tool for homelabs and small-to-medium infrastructure estates.

It is meant to answer three simple questions across many runtime types:

1. What is running right now?
2. What is the newest relevant version?
3. Does this instance need an update?

The long-term goal is broader than "WhatsUp Docker for Docker". `up2date` should support Docker, Docker Compose, VMs, LXCs, physical hosts, Proxmox-managed workloads, and custom applications with user-defined version discovery strategies.

## Project Status

This repository currently contains two runnable prototype components plus the project direction:

- `up2date-agent` for host-side Docker collection
- `up2date-resolver` for latest-version resolution and per-service check publication
- architecture, roadmap, ADRs, examples, and JSON Schemas

The current prototype intentionally stays narrow:

- one `up2date-agent` sidecar per Docker host
- reads nearby containers through the Docker socket
- publishes a retained full-node snapshot to MQTT every minute
- publishes a retained node status summary in the same cycle
- a separate `up2date-resolver` consumes snapshots
- publishes one retained check topic per service with update status
- lets you inspect snapshots and checks directly in MQTT Explorer
- does not require a dedicated backend yet

The snapshot topic is still the MVP source of truth. Status and check topics are derived convenience views built on top of that snapshot contract.

## Recommended Stack

Primary recommendation for the long-term product:

- Core agent + backend API: Go
- Web UI later: TypeScript + React
- Contracts and configuration: language-neutral

Why this remains the recommendation:

- agents and CLI tools benefit from static binaries and easy cross-compilation
- host-side deployment should not require a JVM or Node runtime
- Docker- and infra-oriented integrations are natural in Go
- the web UI will likely evolve at a different pace than the collectors and checkers

If you strongly prefer the Kotlin ecosystem, use Kotlin/JVM instead of Kotlin Multiplatform for the first real implementation.

For this repository's validation prototype, the MQTT agent and resolver are written in Python standard library only. That is a validation vehicle, not a reversal of the long-term runtime decision.

## Documentation Map

- Vision: [docs/vision.md](/Users/alex/Workspace/up2date/docs/vision.md)
- Architecture: [docs/architecture.md](/Users/alex/Workspace/up2date/docs/architecture.md)
- MQTT contract: [docs/mqtt.md](/Users/alex/Workspace/up2date/docs/mqtt.md)
- Roadmap: [docs/roadmap.md](/Users/alex/Workspace/up2date/docs/roadmap.md)
- Language ADR: [docs/adr/0001-language-and-runtime.md](/Users/alex/Workspace/up2date/docs/adr/0001-language-and-runtime.md)
- MQTT-first ADR: [docs/adr/0002-mqtt-first-mvp.md](/Users/alex/Workspace/up2date/docs/adr/0002-mqtt-first-mvp.md)
- Agent guidance: [AGENTS.md](/Users/alex/Workspace/up2date/AGENTS.md)
- Example stacks: [examples/README.md](/Users/alex/Workspace/up2date/examples/README.md)
- Snapshot schema: [schemas/mqtt-node-snapshot.schema.json](/Users/alex/Workspace/up2date/schemas/mqtt-node-snapshot.schema.json)
- Status schema: [schemas/agent-status.schema.json](/Users/alex/Workspace/up2date/schemas/agent-status.schema.json)
- Service check schema: [schemas/service-check.schema.json](/Users/alex/Workspace/up2date/schemas/service-check.schema.json)
- Agent: [agent/README.md](/Users/alex/Workspace/up2date/agent/README.md)
- Resolver: [resolver/README.md](/Users/alex/Workspace/up2date/resolver/README.md)
- Contributing: [CONTRIBUTING.md](/Users/alex/Workspace/up2date/CONTRIBUTING.md)

## Repository Shape

Current repository shape:

```text
agent/
docs/
examples/
resolver/
schemas/
tests/
```

Example-focused directories:

```text
examples/
  compose.yaml
  resolver.compose.yaml
  mosquitto/
```

Likely long-term code layout after the prototype phase:

```text
cmd/
  up2date-agent/
  up2date-server/
internal/
  collectors/
  resolvers/
  comparison/
  reporting/
web/
```

## Quick Start

### Local Demo Stack

From the repository root:

```bash
podman compose -f examples/compose.yaml up --build
```

If you prefer to run from inside the `examples` directory:

```bash
cd examples
podman compose up --build
```

The local demo stack brings up:

- a demo `nginx` container
- a local Mosquitto broker on `localhost:1883`
- `up2date-agent`
- `up2date-resolver`

That gives you an end-to-end loop you can inspect immediately in MQTT Explorer.

Important for Podman users:

- `podman compose` only auto-detects standard filenames like `compose.yaml`
- the agent reads a Docker-compatible API socket from inside the container at `/var/run/docker.sock`
- rootless Podman usually exposes its socket at `${XDG_RUNTIME_DIR}/podman/podman.sock`, so set `UP2DATE_HOST_SOCKET_PATH` before starting, for example:

```bash
UP2DATE_HOST_SOCKET_PATH="${XDG_RUNTIME_DIR}/podman/podman.sock" podman compose -f examples/compose.yaml up --build
```

If the agent logs `Permission denied` when opening `/var/run/docker.sock`, the two usual fixes are:

- mount the correct rootless Podman socket with `UP2DATE_HOST_SOCKET_PATH="${XDG_RUNTIME_DIR}/podman/podman.sock"`
- keep the example's `security_opt: [label=disable]` because Podman documents SELinux label separation as a common cause of bind-mount access failures for host content

For group-based access, Podman also documents `keep-groups` as a workaround when the container needs the caller's supplementary groups, but compose providers do not handle that value consistently. That is why the default local example does not rely on it.

### Resolver-Only Compose

If you already have agents publishing snapshots to an MQTT broker, use the resolver-only example:

```bash
podman compose -f examples/resolver.compose.yaml up --build
```

Update the broker settings inside [examples/resolver.compose.yaml](/Users/alex/Workspace/up2date/examples/resolver.compose.yaml) before deploying it.

## Contracts

The current MQTT contracts are published as JSON Schemas:

- [schemas/mqtt-node-snapshot.schema.json](/Users/alex/Workspace/up2date/schemas/mqtt-node-snapshot.schema.json)
- [schemas/agent-status.schema.json](/Users/alex/Workspace/up2date/schemas/agent-status.schema.json)
- [schemas/service-check.schema.json](/Users/alex/Workspace/up2date/schemas/service-check.schema.json)

The snapshot contract remains the primary source of truth. Status and check payloads are intentionally slimmer, derived messages.

## Container Image Publishing

If you publish container images later, prefer neutral project-owned registry paths such as:

- `ghcr.io/your-org/up2date-agent`
- `ghcr.io/your-org/up2date-resolver`

Before publishing, pass `VERSION`, `VCS_REF`, and `BUILD_DATE` build arguments so OCI metadata stays accurate in both images.

## GitHub Publishing Readiness

This repository now includes:

- runnable example stacks under [examples/README.md](/Users/alex/Workspace/up2date/examples/README.md)
- JSON Schemas for snapshot, status, and check payloads
- contract tests under [tests/test_contracts.py](/Users/alex/Workspace/up2date/tests/test_contracts.py)
- a basic GitHub Actions test workflow at [.github/workflows/ci.yml](/Users/alex/Workspace/up2date/.github/workflows/ci.yml)
- contribution guidance in [CONTRIBUTING.md](/Users/alex/Workspace/up2date/CONTRIBUTING.md)

One deliberate publishing decision is still left open: the repository license. That is a project policy choice, so it should be chosen explicitly rather than guessed.
