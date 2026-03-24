# Agent

This directory contains the first validation implementation of `up2date-agent`.

It is intentionally small and dependency-free:

- Python standard library only
- reads Docker state from the local Docker socket
- publishes retained MQTT snapshots and status summaries
- keeps current-state collection separate from latest-version resolution

## Why This Exists

The goal is to validate the MQTT snapshot contract and Docker collection behavior before committing to the long-term production runtime.

The production runtime recommendation remains Go. This implementation exists to make the prototype cheap to iterate on while keeping the domain model explicit.

`UP2DATE_NODE_ID` is the stable technical identity for the MQTT topic path. `UP2DATE_NODE_NAME` is an optional human-friendly label that appears inside the snapshot payload.

## Environment Variables

- `UP2DATE_NODE_ID`
- `UP2DATE_NODE_NAME`
- `UP2DATE_MQTT_HOST`
- `UP2DATE_MQTT_PORT`
- `UP2DATE_MQTT_USERNAME`
- `UP2DATE_MQTT_PASSWORD`
- `UP2DATE_MQTT_TOPIC_PREFIX`
- `UP2DATE_INTERVAL_SECONDS`
- `UP2DATE_DOCKER_SOCKET`
- `UP2DATE_INCLUDE_STOPPED`
- `UP2DATE_RETAIN_MESSAGES`
- `UP2DATE_STDOUT_ONLY`
- `UP2DATE_ONE_SHOT`
- `UP2DATE_DOCKER_FIXTURE_PATH`
- `UP2DATE_EXCLUDE_SELF`
- `UP2DATE_EXCLUDE_LABELS`

## Topics

- publishes to `up2date/nodes/<node-id>/snapshot`
- publishes to `up2date/nodes/<node-id>/status`

Related contracts:

- [schemas/mqtt-node-snapshot.schema.json](/Users/alex/Workspace/up2date/schemas/mqtt-node-snapshot.schema.json)
- [schemas/agent-status.schema.json](/Users/alex/Workspace/up2date/schemas/agent-status.schema.json)

## Local Dry Run

```bash
UP2DATE_NODE_ID=lab-01 \
UP2DATE_STDOUT_ONLY=true \
UP2DATE_ONE_SHOT=true \
UP2DATE_DOCKER_FIXTURE_PATH=agent/fixtures/docker_containers.json \
python3 agent/app/up2date_agent.py
```

## Compose Run

For the local end-to-end demo stack:

```bash
podman compose -f examples/compose.yaml up --build
```

If you use a non-default Podman socket path, map it into the container via:

```bash
UP2DATE_HOST_SOCKET_PATH="${XDG_RUNTIME_DIR}/podman/podman.sock" podman compose -f examples/compose.yaml up --build
```

If you want to run from inside the `examples` directory instead:

```bash
cd examples
podman compose up --build
```

The demo stack includes:

- a demo app
- a local Mosquitto broker
- `up2date-agent`
- `up2date-resolver`

If the agent reports `Permission denied` for `/var/run/docker.sock`, the common Podman fixes are:

- use your rootless Podman socket with `UP2DATE_HOST_SOCKET_PATH="${XDG_RUNTIME_DIR}/podman/podman.sock"`
- keep `security_opt: [label=disable]` on the agent service for SELinux-heavy hosts
- if socket access depends on supplementary groups, `keep-groups` may help in Podman-native flows, but external compose providers can misinterpret it as a literal group name

Containers can be excluded from the snapshot with labels like `up2date.ignore=true`. The default exclude selectors are:

- `up2date.ignore=true`
- `com.up2date.ignore=true`

## Test Run

```bash
python3 -m unittest discover -s agent/tests -v
python3 -m unittest discover -s tests -v
```
