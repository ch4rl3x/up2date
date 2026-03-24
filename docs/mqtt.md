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
- `up2date/nodes/<node-id>/checks/<service>`

All of these topics should be published as retained messages.

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

## Check Payload

The resolver publishes one compact check payload per service.

The snapshot remains the detailed source of truth. The check topic is intentionally slim so it is easier to consume in MQTT Explorer, Home Assistant, and later UIs.

Example topic:

- `up2date/nodes/local-dev-node/checks/app`

Example payload:

```json
{
  "schema_version": 1,
  "kind": "service_check",
  "node_id": "local-dev-node",
  "node_name": "Local Dev Node",
  "service_name": "app",
  "observed_at": "2026-03-24T18:42:30Z",
  "image_name": "docker.io/library/nginx",
  "current_version": "1.27-alpine",
  "latest_version": "1.28-alpine",
  "status": "outdated",
  "update_available": true
}
```

Notes:

- `latest_version` is only published when the resolver finds a compatible version track with high confidence.
- if a registry contains mixed tag schemes, the resolver prefers returning `unknown` over publishing a misleading `latest_version`.
- use the snapshot topic if you need debug metadata such as image tag, version source, or exact observation details.

## Publication Rules

- Publish every 60 seconds by default.
- Publish the full node snapshot each cycle.
- Publish the status summary in the same cycle.
- Publish one retained check message per service whenever a new snapshot is processed.
- Use retained messages so new observers immediately see the last known state.
- Treat the snapshot topic as the source of truth.

## Freshness Rules

Until a real backend exists, freshness is human-observed in MQTT Explorer.

When a backend is added later, it should treat a node as stale if no fresh snapshot arrives after multiple expected intervals.

## Deliberate MVP Omissions

- no change history
- no update decision logic yet
- no persistent database
- no per-container topics as the source of truth

Those will come after the snapshot contract is stable.
