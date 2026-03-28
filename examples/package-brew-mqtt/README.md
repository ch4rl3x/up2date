# package-brew-mqtt

Startet einen lokalen MQTT-Broker per Compose, baut `up2date` lokal als Binary und startet dieses Binary danach auf macOS mit Homebrew-Collector und statischer `config.yml`. Der Resolver wird automatisch als `brew_formula` abgeleitet.

Das eingecheckte Beispiel-Config-File liegt unter [config.yml](/Users/alex/Workspace/up2date/examples/package-brew-mqtt/config.yml).

Standard:

```bash
./examples/package-brew-mqtt/run.sh
```

Dauerlauf statt One-Shot:

```bash
./examples/package-brew-mqtt/run.sh --continuous
```

Broker stoppen:

```bash
podman compose -f examples/package-brew-mqtt/compose.yml down
```
