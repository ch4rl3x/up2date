# Roadmap

## Current Repository Status

This repository already contains prototype implementations for the first two delivery phases:

- Phase 1: MQTT validation agent
- Phase 2: MQTT resolver

The next major delivery focus is the backend subscriber and UI surface that consume the same contracts.

## Phase 0: Foundation

- freeze the product model
- choose the runtime and delivery strategy
- define the MQTT snapshot contract
- document MVP scope and non-goals

## Phase 1: MQTT Validation MVP

Prototype status in this repository:

- local agent implementation exists
- Docker Engine scanning exists
- image tag and version label detection exists
- full node snapshot publishing exists
- node status summary publishing exists
- local inspection in MQTT Explorer exists

Success criteria:

- a single Docker host publishes a stable retained snapshot every minute
- the snapshot is easy to inspect from a laptop
- running and stopped containers are distinguishable

## Phase 2: MQTT Resolver

Prototype status in this repository:

- resolver consumes MQTT snapshots
- registry tag lookup exists for Docker-style registries
- current-vs-latest comparison exists for compatible numeric tracks
- per-service retained check topics exist

Still intentionally limited:

- no persistent backend read model yet
- no user-configurable comparator strategies yet
- no non-registry resolvers yet

## Phase 3: Backend Subscriber

- build a subscriber that consumes snapshots and checks
- expose the last known state through an API
- add freshness and stale-node rules
- add a basic list view

## Phase 4: Generic Collectors

- add SSH command collectors
- add HTTP regex collectors
- support custom parsing rules
- expose collector health and confidence

## Phase 5: Mixed Infrastructure

- add Proxmox-native inventory options
- add VM and LXC tracking models
- introduce host-level grouping and ownership metadata

## Phase 6: Product Maturity

- notifications
- richer UI filters and search
- change history
- policy rules such as "critical if behind by more than N releases"
- optional automation hooks

## Deferred Until Clearly Needed

- automatic updates
- bidirectional configuration sync
- marketplace-style plugin loading
- multi-tenant complexity
