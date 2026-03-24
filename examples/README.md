# Examples

This directory intentionally contains more than one Compose file because the repository has two distinct usage modes.

## Files

- [compose.yaml](/Users/alex/Workspace/up2date/examples/compose.yaml): local end-to-end demo with app, broker, agent, and resolver
- [resolver.compose.yaml](/Users/alex/Workspace/up2date/examples/resolver.compose.yaml): standalone resolver deployment for an existing broker
- [mosquitto/mosquitto.conf](/Users/alex/Workspace/up2date/examples/mosquitto/mosquitto.conf): local broker config for the demo stack

## Local Demo

From the repository root:

```bash
podman compose -f examples/compose.yaml up --build
```

Or from inside this directory:

```bash
podman compose up --build
```

This stack is meant for quick local validation and MQTT Explorer inspection.

## Resolver-Only Deployment

Use the resolver-only file when agents already publish snapshots into a broker you manage elsewhere:

```bash
podman compose -f examples/resolver.compose.yaml up --build
```

Update the broker hostname and optional credentials in that file before deployment.
