# Vision

## Problem

Infrastructure owners often know that software exists, but not which exact version is currently deployed across all environments.

This is especially messy in homelabs and mixed estates where workloads may live in:

- Docker or Docker Compose
- LXC containers
- VMs
- physical hosts
- Proxmox-managed systems
- custom applications exposing version data through ad hoc endpoints

Existing tools usually solve only one slice:

- package updates on a host
- image updates for containers
- release monitoring for repositories

`up2date` aims to unify those concerns under one model.

## Product Thesis

Users do not primarily need "one more updater". They need a trustworthy inventory of version drift.

Long-term that means:

- flexible current-version collection
- flexible latest-version resolution
- explicit comparison strategies
- explainable results
- central visibility

The current prototype validates the first practical loop of that model:

- discover Docker containers locally
- normalize their runtime state into one node snapshot
- publish snapshots and status summaries to MQTT on a fixed cadence
- resolve newer registry tags in a separate process
- publish retained per-service check topics

## Goals

- Track deployed versions across heterogeneous runtime types.
- Support both push and pull collection patterns.
- Allow custom integrations without forking the core.
- Preserve provenance for every detected version.
- Be useful without a custom backend in the earliest phase.
- Grow into a web-configurable system without rewriting the domain model.

## Non-Goals

- Automatic patching in the MVP
- Full configuration management
- Replacing monitoring or CMDB tooling
- Universal package-manager support from day one
- Remote execution on arbitrary machines without explicit operator intent
- Building the final backend before the snapshot contract is validated

## Users

Primary users:

- homelab operators
- self-hosters
- small infrastructure teams
- platform engineers with mixed legacy and containerized workloads

## MVP Definition

The current MVP focuses on one narrow but high-value loop:

1. Observe Docker deployments on a node.
2. Extract image tags, version labels, and running state.
3. Publish one normalized snapshot plus a small status summary to MQTT every minute.
4. Consume those snapshots in a separate resolver process.
5. Publish one retained per-service check result for easy inspection.
6. Use the learned snapshot contract as the basis for the first backend.

Snapshot topics remain the source of truth. Check topics exist to make the update outcome easier to inspect and consume.

## Product Principles

- Explain every result.
- Be conservative with claims when data quality is weak.
- Prefer explicit configuration over hidden magic.
- Default to safe collection patterns.
- Make generic extension possible without making simple use cases hard.
