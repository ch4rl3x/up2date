# Resolver

This directory contains the first validation implementation of `up2date-resolver`.

It is intentionally small and dependency-free:

- Python standard library only
- subscribes to MQTT node snapshots
- resolves newer versions from Docker registries
- publishes one retained check topic per service

## Why This Exists

The resolver keeps latest-version logic separate from the host-side agent.

The agent answers "what is running right now?". The resolver answers "is a newer version available for this service?".

## Environment Variables

- `UP2DATE_MQTT_HOST`
- `UP2DATE_MQTT_PORT`
- `UP2DATE_MQTT_USERNAME`
- `UP2DATE_MQTT_PASSWORD`
- `UP2DATE_MQTT_TOPIC_PREFIX`
- `UP2DATE_RETAIN_MESSAGES`
- `UP2DATE_STDOUT_ONLY`
- `UP2DATE_ONE_SHOT`
- `UP2DATE_SNAPSHOT_FIXTURE_PATH`
- `UP2DATE_REGISTRY_FIXTURE_PATH`
- `UP2DATE_REGISTRY_TIMEOUT_SECONDS`
- `UP2DATE_INSECURE_REGISTRIES`
- `UP2DATE_MQTT_READ_TIMEOUT_SECONDS`

## Topics

- subscribes to `up2date/nodes/+/snapshot`
- publishes to `up2date/nodes/<node>/checks/<service>`

Related contract:

- [schemas/service-check.schema.json](/Users/alex/Workspace/up2date/schemas/service-check.schema.json)

## Local Dry Run

```bash
UP2DATE_STDOUT_ONLY=true \
UP2DATE_ONE_SHOT=true \
UP2DATE_SNAPSHOT_FIXTURE_PATH=resolver/fixtures/node_snapshot.json \
UP2DATE_REGISTRY_FIXTURE_PATH=resolver/fixtures/registry_tags.json \
python3 resolver/app/up2date_resolver.py
```

## Compose Runs

For the local end-to-end demo stack:

```bash
podman compose -f examples/compose.yaml up --build
```

For a standalone resolver deployment that connects to an existing broker:

```bash
podman compose -f examples/resolver.compose.yaml up --build
```

The resolver-only example is intentionally separate so deployment-oriented settings do not clutter the local demo stack.

## Container Image Notes

If you later publish a project-owned image, prefer a neutral path such as:

- `ghcr.io/your-org/up2date-resolver`

Example local build command with OCI metadata:

```bash
podman build \
  --build-arg VERSION=0.1.0-alpha.1 \
  --build-arg VCS_REF="$(git rev-parse --short HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t ghcr.io/your-org/up2date-resolver:0.1.0-alpha.1 \
  resolver
```

If you build on Apple Silicon and deploy to `amd64` Linux hosts, keep using `--platform linux/amd64` until you publish a proper multi-arch manifest.

## Test Run

```bash
python3 -m unittest discover -s resolver/tests -v
python3 -m unittest discover -s tests -v
```
