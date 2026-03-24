# up2date

`up2date` is a version observability tool for homelabs and small-to-medium infrastructure estates.

It is meant to answer three simple questions across many runtime types:

1. What is running right now?
2. What is the newest relevant version?
3. Does this instance need an update?

The long-term goal is broader than "WhatsUp Docker for Docker". `up2date` should support Docker, Docker Compose, VMs, LXCs, physical hosts, Proxmox-managed workloads, and custom applications with user-defined version discovery strategies.

## Project Status

This repository currently contains two runnable components plus the project direction:

- the project direction and decision records
- `up2date-agent` for host-side Docker collection
- `up2date-resolver` for version resolution and check topic enrichment

The current MVP target is intentionally narrow:

- one `up2date-agent` sidecar per Docker host
- reads nearby containers through the Docker socket
- publishes a full host snapshot to MQTT every minute
- a separate `up2date-resolver` consumes those snapshots
- publishes one check topic per service with update status
- lets you inspect those values directly in MQTT Explorer
- no dedicated backend yet

This keeps the first loop focused on the risky parts: container discovery, topic design, payload quality, and freshness semantics.

## Recommended Stack

Primary recommendation for the long-term product:

- Core agent + backend API: Go
- Web UI later: TypeScript + React
- Contracts and configuration: language-neutral

Why this is the long-term recommendation:

- agents and CLI tools benefit from static binaries and easy cross-compilation
- host-side deployment should not require a JVM or Node runtime
- Docker- and infra-oriented integrations are natural in Go
- the web UI will likely evolve at a different pace than the collectors and checkers

If you strongly prefer the Kotlin ecosystem, use Kotlin/JVM instead of Kotlin Multiplatform for the first real implementation.

For this repository's first prototype, the MQTT agent and resolver are written in Python standard library only. That is a validation vehicle, not a reversal of the long-term runtime decision.

## Kotlin Multiplatform Verdict

Kotlin Multiplatform is technically possible, but it is not the recommended starting point.

The main reason is not language quality. The problem is ecosystem friction:

- many infra libraries are JVM-first, not commonMain-first
- SSH, Docker, registry, YAML, Proxmox, and HTML parsing support is uneven across targets
- the operational payoff of code sharing between backend, agent, and UI is limited here
- KMP would add build and target complexity before the product model is stable

Short version:

- `KMP`: possible, not recommended for MVP
- `Kotlin/JVM`: viable if team fit matters most
- `Go`: best default for this product shape

See [docs/adr/0001-language-and-runtime.md](/Users/alex/Workspace/up2date/docs/adr/0001-language-and-runtime.md) for the runtime decision and [docs/adr/0002-mqtt-first-mvp.md](/Users/alex/Workspace/up2date/docs/adr/0002-mqtt-first-mvp.md) for the current MVP transport decision.

## Documentation Map

- Vision: [docs/vision.md](/Users/alex/Workspace/up2date/docs/vision.md)
- Architecture: [docs/architecture.md](/Users/alex/Workspace/up2date/docs/architecture.md)
- MQTT contract: [docs/mqtt.md](/Users/alex/Workspace/up2date/docs/mqtt.md)
- Roadmap: [docs/roadmap.md](/Users/alex/Workspace/up2date/docs/roadmap.md)
- Language ADR: [docs/adr/0001-language-and-runtime.md](/Users/alex/Workspace/up2date/docs/adr/0001-language-and-runtime.md)
- MVP transport ADR: [docs/adr/0002-mqtt-first-mvp.md](/Users/alex/Workspace/up2date/docs/adr/0002-mqtt-first-mvp.md)
- Agent guidance: [AGENTS.md](/Users/alex/Workspace/up2date/AGENTS.md)
- Compose quick-start: [examples/compose.yaml](/Users/alex/Workspace/up2date/examples/compose.yaml)
- Snapshot schema: [schemas/mqtt-node-snapshot.schema.json](/Users/alex/Workspace/up2date/schemas/mqtt-node-snapshot.schema.json)
- Service check schema: [schemas/service-check.schema.json](/Users/alex/Workspace/up2date/schemas/service-check.schema.json)
- Agent: [agent/README.md](/Users/alex/Workspace/up2date/agent/README.md)
- Resolver: [resolver/README.md](/Users/alex/Workspace/up2date/resolver/README.md)

## Planned Repository Shape

This is the current repository shape:

```text
docs/
examples/
agent/
resolver/
schemas/
```

This is the likely long-term code layout after the prototype phase:

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

## First MVP Boundary

The current MVP slice should do only this:

- scan Docker workloads on a host
- identify image names, tags, version labels, and running state
- publish one full retained snapshot per node to MQTT every minute
- consume snapshots in a separate resolver service
- publish per-service update checks to MQTT
- inspect the data directly in MQTT Explorer
- refine topic and payload shape before a backend exists

The first backend should subscribe to the same MQTT snapshot contract instead of inventing a second ingestion model.

## Quick Start

From the repository root:

```bash
podman compose -f examples/compose.yaml up
```

If you prefer to run from inside the `examples` directory:

```bash
cd examples
podman compose up
```

Important for Podman users:

- `podman compose` only auto-detects standard filenames like `compose.yaml`
- the agent reads a Docker-compatible API socket from inside the container at `/var/run/docker.sock`
- rootless Podman usually exposes its socket at `${XDG_RUNTIME_DIR}/podman/podman.sock`, so set `UP2DATE_HOST_SOCKET_PATH` before starting, for example:

```bash
UP2DATE_HOST_SOCKET_PATH="${XDG_RUNTIME_DIR}/podman/podman.sock" podman compose -f examples/compose.yaml up --build
```

The example app image is now `docker.io/library/nginx:1.27-alpine` so the stack does not depend on a private or placeholder registry image.

If the agent logs `Permission denied` when opening `/var/run/docker.sock`, the two usual fixes are:

- mount the correct rootless Podman socket with `UP2DATE_HOST_SOCKET_PATH="${XDG_RUNTIME_DIR}/podman/podman.sock"`
- keep the example's `security_opt: [label=disable]` because Podman documents SELinux label separation as a common cause of bind-mount access failures for host content

For group-based access, Podman also documents `keep-groups` as a workaround when the container needs the caller's supplementary groups, but your current `podman compose` setup is using an external `docker-compose` provider, and that provider may treat `keep-groups` as a literal group name instead of a Podman special value. That is why the default example no longer sets it.

The current example file is geared toward resolver deployment with a published image and placeholder MQTT settings. It also keeps commented local stack snippets for:

- a demo app
- Mosquitto
- `up2date-agent`
- a locally built `up2date-resolver`

## Published Alpha Images

The current Docker Hub naming plan is:

- `docker.io/ch4rl3x/up2date-agent`
- `docker.io/ch4rl3x/up2date-resolver`

For Linux server tests, prefer explicit alpha tags and build `amd64` images when publishing from Apple Silicon.
