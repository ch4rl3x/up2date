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

## Local Dry Run

```bash
UP2DATE_STDOUT_ONLY=true \
UP2DATE_ONE_SHOT=true \
UP2DATE_SNAPSHOT_FIXTURE_PATH=resolver/fixtures/node_snapshot.json \
UP2DATE_REGISTRY_FIXTURE_PATH=resolver/fixtures/registry_tags.json \
python3 resolver/app/up2date_resolver.py
```

## Compose Run

From the repository root:

```bash
podman compose -f examples/compose.yaml up
```

If you want to build the local resolver code instead of pulling the published image, switch the commented `build:` block back on in [examples/compose.yaml](/Users/alex/Workspace/up2date/examples/compose.yaml).

## Published Image

Planned Docker Hub image name:

- `docker.io/ch4rl3x/up2date-resolver`

Suggested first alpha tag:

- `docker.io/ch4rl3x/up2date-resolver:0.1.0-alpha.1`

Build an `amd64` image for your Linux hosts:

```bash
podman build \
  --platform linux/amd64 \
  --build-arg VERSION=0.1.0-alpha.1 \
  -t docker.io/ch4rl3x/up2date-resolver:0.1.0-alpha.1 \
  resolver
```

Push it to Docker Hub:

```bash
podman push docker.io/ch4rl3x/up2date-resolver:0.1.0-alpha.1
```

Optional moving alpha tag:

```bash
podman tag docker.io/ch4rl3x/up2date-resolver:0.1.0-alpha.1 docker.io/ch4rl3x/up2date-resolver:alpha
podman push docker.io/ch4rl3x/up2date-resolver:alpha
```

If you build on Apple Silicon and deploy to `amd64` Linux hosts, keep using `--platform linux/amd64` until we publish a proper multi-arch manifest.

## Deploy With A Published Image

```yaml
services:
  up2date-resolver:
    image: docker.io/ch4rl3x/up2date-resolver:0.1.0-alpha.1
    environment:
      UP2DATE_MQTT_HOST: mqtt.example.internal
      UP2DATE_MQTT_PORT: 1883
      UP2DATE_MQTT_USERNAME: change-me
      UP2DATE_MQTT_PASSWORD: change-me
      UP2DATE_RETAIN_MESSAGES: "true"
    restart: unless-stopped
```

## Test Run

```bash
python3 -m unittest discover -s resolver/tests -v
```
