# Contributing

Thanks for contributing to `up2date`.

## Ground Rules

- Keep current-state collection separate from latest-version resolution.
- Preserve provenance in emitted observations and payloads.
- Prefer the smallest abstraction that keeps future plugin boundaries clean.
- If behavior or project direction changes, update the docs and contracts in the same change.

The canonical direction and terminology live in [AGENTS.md](/Users/alex/Workspace/up2date/AGENTS.md).

## Local Development

Run the local demo stack:

```bash
podman compose -f examples/compose.yaml up --build
```

Run the standalone resolver example against an existing broker:

```bash
podman compose -f examples/resolver.compose.yaml up --build
```

## Tests

Run all current test suites before opening a PR:

```bash
python3 -m unittest discover -s agent/tests -v
python3 -m unittest discover -s resolver/tests -v
python3 -m unittest discover -s tests -v
```

## Documentation And Contracts

If you change payload shape, topic behavior, or project direction, update the related files in the same PR:

- [README.md](/Users/alex/Workspace/up2date/README.md)
- [docs/vision.md](/Users/alex/Workspace/up2date/docs/vision.md)
- [docs/architecture.md](/Users/alex/Workspace/up2date/docs/architecture.md)
- [docs/mqtt.md](/Users/alex/Workspace/up2date/docs/mqtt.md)
- [docs/roadmap.md](/Users/alex/Workspace/up2date/docs/roadmap.md)
- [docs/adr/0001-language-and-runtime.md](/Users/alex/Workspace/up2date/docs/adr/0001-language-and-runtime.md)
- [docs/adr/0002-mqtt-first-mvp.md](/Users/alex/Workspace/up2date/docs/adr/0002-mqtt-first-mvp.md)
- [schemas/mqtt-node-snapshot.schema.json](/Users/alex/Workspace/up2date/schemas/mqtt-node-snapshot.schema.json)
- [schemas/agent-status.schema.json](/Users/alex/Workspace/up2date/schemas/agent-status.schema.json)
- [schemas/service-check.schema.json](/Users/alex/Workspace/up2date/schemas/service-check.schema.json)

## Pull Requests

Small, focused PRs are easier to review than broad refactors.

Before opening a PR:

- keep examples runnable
- keep schemas aligned with emitted payloads
- keep tests green
- note any deliberate follow-up work explicitly
