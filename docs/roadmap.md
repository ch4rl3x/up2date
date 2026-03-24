# Roadmap

## Phase 0: Foundation

- freeze the product model
- choose the runtime and delivery strategy
- define the MQTT snapshot contract
- document MVP scope and non-goals

## Phase 1: MQTT Validation MVP

- implement a local agent or CLI
- support Docker Engine scanning
- detect current image tags and version labels
- publish full node snapshots to MQTT
- inspect results in MQTT Explorer
- refine the topic and payload shape

Success criteria:

- a single Docker host publishes a stable retained snapshot every minute
- the snapshot is easy to inspect from a laptop
- running and stopped containers are distinguishable

## Phase 2: Backend Subscriber

- build a subscriber that consumes the MQTT snapshot
- expose the last known state through an API
- add freshness and stale-node rules
- add a basic list view

## Phase 3: Version Resolution

- resolve latest versions from registries or APIs
- compare current and latest versions
- mark workloads as outdated or unknown
- keep the comparison model explicit

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
