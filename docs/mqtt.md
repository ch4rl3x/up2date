# MQTT Contract

This document defines the current MVP transport for `up2date`.

## Goal

Validate the host-side collection model before a backend exists.

The agent publishes Docker state snapshots to MQTT. For now, humans inspect those values with MQTT Explorer or similar tooling.

## Why MQTT For The MVP

- easy to observe from a laptop without building a backend first
- good fit for repeated state publication
- simple path to add a subscriber-based backend later
- works well in homelab environments that already use a broker

## Why Periodic Snapshots Instead Of One-Time Start Events

The MVP publishes a full node snapshot every minute because that is more robust than a one-time startup event:

- broker or consumer restarts recover naturally
- manual container changes show up without waiting for a restart of the agent
- freshness can be inferred from `observed_at`
- topic debugging is simpler when every message is self-contained

## Topic Design

Current topics:

- `up2date/nodes/<node-id>/snapshot`
- `up2date/nodes/<node-id>/status`

Both topics should be published as retained messages.

## Snapshot Payload

The snapshot is the primary contract.

Example:

```json
{
  "schema_version": 1,
  "kind": "docker_node_snapshot",
  "agent_id": "docker-host-01",
  "node_name": "Docker Host 01",
  "observed_at": "2026-03-24T18:42:00Z",
  "services": [
    {
      "container_id": "8f69a0d8d5d4",
      "container_name": "media-plex-1",
      "service_name": "plex",
      "project_name": "media",
      "image": "lscr.io/linuxserver/plex:1.41.8",
      "image_name": "lscr.io/linuxserver/plex",
      "image_tag": "1.41.8",
      "detected_version": "1.41.8",
      "detected_version_source": "image_tag",
      "state": "running",
      "running": true,
      "status": "Up 23 minutes",
      "observed_via": "docker_engine"
    }
  ]
}
```

Notes:

- `agent_id` is the stable technical identity used in the topic path.
- `node_name` is an optional human-friendly label and should be set explicitly.
- `image_tag` and `detected_version` are not always the same concept. If no version label exists, `detected_version` currently falls back to the image tag.
- containers can be excluded from snapshots with labels such as `up2date.ignore=true`.

## Status Payload

The status topic is a small summary derived from the snapshot.

Example:

```json
{
  "schema_version": 1,
  "kind": "agent_status",
  "agent_id": "docker-host-01",
  "observed_at": "2026-03-24T18:42:00Z",
  "service_count": 12,
  "running_service_count": 11
}
```

## Publication Rules

- Publish every 60 seconds by default.
- Publish the full node snapshot each cycle.
- Publish the status summary in the same cycle.
- Use retained messages so new observers immediately see the last known state.
- Treat the snapshot topic as the source of truth.

## Freshness Rules

Until a real backend exists, freshness is human-observed in MQTT Explorer.

When a backend is added later, it should treat a node as stale if no fresh snapshot arrives after multiple expected intervals.

## Deliberate MVP Omissions

- no change history
- no version resolution against registries yet
- no update decision logic yet
- no persistent database
- no per-container topics as the source of truth

Those will come after the snapshot contract is stable.
