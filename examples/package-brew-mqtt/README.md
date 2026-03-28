# package-brew-mqtt

Startet einen lokalen MQTT-Broker per Compose und fuehrt `up2date` danach auf macOS mit Homebrew-Collector und `brew_formula`-Resolver aus.

Standard:

```bash
./examples/package-brew-mqtt/run.sh
```

Mit explizitem Paket:

```bash
UP2DATE_COLLECTOR_PACKAGE_NAMES=samba ./examples/package-brew-mqtt/run.sh
```

Dauerlauf statt One-Shot:

```bash
./examples/package-brew-mqtt/run.sh --continuous
```

Broker stoppen:

```bash
podman compose -f examples/package-brew-mqtt/compose.yml down
```
