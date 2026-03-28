# up2date

`up2date` liest aktuell Docker-Container von `docker.sock` oder lokal installierte OS-Pakete, prueft verfuegbare Versionen ueber einen Resolver und published das Ergebnis per MQTT.

Der Collector liefert die aktuellen Fakten eines Workloads, inklusive `artifact_name` und `artifact_ref`. Der Resolver arbeitet auf dem Referenzfeld und reichert nur um Upstream-Informationen an. Der Publisher published nur die bereits vorbereiteten Felder.

Die Konfiguration passiert ausschliesslich ueber Umgebungsvariablen.

## Nutzung mit Docker Compose

Direkt nutzbare Beispiele:

- Docker-Collector plus MQTT: [examples/docker-mqtt/compose.yml](/Users/alex/Workspace/up2date/examples/docker-mqtt/compose.yml)
- macOS Homebrew-Collector plus MQTT: [examples/package-brew-mqtt/run.sh](/Users/alex/Workspace/up2date/examples/package-brew-mqtt/run.sh)

Zum direkten Testen des Docker-Collectors kannst du dieses Beispiel starten:

```bash
docker compose -f examples/docker-mqtt/compose.yml up --build -d
```

Wenn du die Anwendung in deine eigene Compose-Datei uebernimmst, reicht dieser Dienst:

```yaml
services:
  mqtt:
    image: eclipse-mosquitto:2
    ports:
      - "1883:1883"

  up2date:
    build: .
    depends_on:
      - mqtt
    restart: unless-stopped
    environment:
      UP2DATE_NODE_ID: docker-host-01
      UP2DATE_INTERVAL: 1m

      UP2DATE_COLLECTOR_TYPE: docker
      UP2DATE_RESOLVER_TYPE: docker_hub

      UP2DATE_PUBLISHER_TYPE: mqtt
      UP2DATE_PUBLISHER_MQTT_HOST: mqtt
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

Starten:

```bash
docker compose up --build -d
```

## Wichtige Variablen

Allgemein:
- `UP2DATE_NODE_ID`
- `UP2DATE_INTERVAL`

Collector:
- `UP2DATE_COLLECTOR_TYPE`: `docker` oder `package`

Resolver:
- `UP2DATE_RESOLVER_TYPE`: `docker_hub`, `brew_formula` oder `none`
- Default je Collector:
  - `docker` -> `docker_hub`
  - `package` -> `none`

Collector `docker`:
- `UP2DATE_COLLECTOR_DOCKER_INCLUDE_STOPPED`
- `UP2DATE_COLLECTOR_DOCKER_EXCLUDE_SELF`
- `UP2DATE_COLLECTOR_DOCKER_EXCLUDE_LABELS`

Collector `package`:
- `UP2DATE_COLLECTOR_PACKAGE_MANAGER` optional, Default `dpkg`, aktuell `dpkg` oder `brew`
- `UP2DATE_COLLECTOR_PACKAGE_NAMES` CSV-Liste, z. B. `samba,wsdd2`

Publisher:
- `UP2DATE_PUBLISHER_TYPE`: aktuell nur `mqtt`

Publisher `mqtt`:
- `UP2DATE_PUBLISHER_MQTT_HOST`
- `UP2DATE_PUBLISHER_MQTT_PORT` optional, Default `1883`
- `UP2DATE_PUBLISHER_MQTT_USERNAME`
- `UP2DATE_PUBLISHER_MQTT_PASSWORD`
- `UP2DATE_PUBLISHER_MQTT_TOPIC_PREFIX` optional, Default `up2date`
- `UP2DATE_PUBLISHER_MQTT_CLIENT_ID_PREFIX`
- `UP2DATE_PUBLISHER_MQTT_CONNECT_TIMEOUT`
- `UP2DATE_PUBLISHER_MQTT_RETAIN`

## MQTT

Es werden pro Dienst einfache Feld-Topics geschrieben:

- `up2date/<node_id>/<service_name>/artifact_name`
- `up2date/<node_id>/<service_name>/current_version`
- `up2date/<node_id>/<service_name>/latest_version`
- `up2date/<node_id>/<service_name>/latest_version_url`
- `up2date/<node_id>/<service_name>/observed_at`
- `up2date/<node_id>/<service_name>/check_status`

Beispielwerte:

```text
up2date/docker-host-01/nginx/artifact_name = nginx
up2date/docker-host-01/nginx/current_version = 1.27-alpine
up2date/docker-host-01/nginx/latest_version = 1.29-alpine
up2date/docker-host-01/nginx/latest_version_url = https://hub.docker.com/_/nginx/tags?name=1.29-alpine
up2date/docker-host-01/nginx/observed_at = 2026-03-27T20:44:59Z
up2date/docker-host-01/nginx/check_status = outdated
```

## LXC mit direkt installierter Software

Fuer LXC-Container ist die bevorzugte Richtung, `up2date` im jeweiligen Container selbst laufen zu lassen. Das passt zu den Guardrails: Current-State wird lokal gesammelt, ohne zentrale privilegierte LXC-Host-Zugriffe.

Fuer einen Debian-/Ubuntu-basierten Samba-Container reicht zunaechst diese Konfiguration:

```bash
UP2DATE_NODE_ID=lxc-samba-01
UP2DATE_INTERVAL=1m

UP2DATE_COLLECTOR_TYPE=package
UP2DATE_COLLECTOR_PACKAGE_MANAGER=dpkg
UP2DATE_COLLECTOR_PACKAGE_NAMES=samba

UP2DATE_PUBLISHER_TYPE=mqtt
UP2DATE_PUBLISHER_MQTT_HOST=192.168.1.20
```

Der `package`-Collector fragt dann lokal per `dpkg-query` ab und erzeugt eine Observation fuer `samba` mit:

- `artifact_type = os_package`
- `artifact_name = samba`
- `artifact_ref = dpkg:samba`
- `current_version_source = dpkg-query`
- `observed_via = local_package_manager`

Der Default-Resolver fuer `package` ist absichtlich `none`. Damit trennen wir die Frage "was ist installiert?" sauber von "was waere die passende neuere Version?".

Wenn `samba` nicht installiert ist, bleibt `current_version` leer. Das MQTT-Topic fuer `current_version` wird dadurch geloescht, waehrend `observed_at` und `check_status=unknown` weiter den aktuellen Beobachtungsstand zeigen.

## Integration `package`

Vor dem Hinzufuegen dieser Integration wurden die Guardrails fuer neue Integrationen konkretisiert:

1. Input:
ein lokaler Paketmanager wie `dpkg-query` oder `brew info --formula --json=v2` und eine CSV-Liste ueber `UP2DATE_COLLECTOR_PACKAGE_NAMES`.
2. Normalized output:
pro Paket eine Observation mit `artifact_type=os_package`, `artifact_ref=dpkg:<paketname>`, `current_version`, Provenance-Feldern und Paket-Metadaten in `attributes`.
3. Secrets:
fuer die lokale Paketabfrage keine; nur MQTT-Credentials bleiben relevant.
4. Comparator strategy:
aktuell keiner, deshalb Default-Resolver `none`; spaeter kann ein eigener Debian-/APT-Resolver mit passender Version-Comparison folgen.
5. Visible failure modes:
fehlendes Paket wird als Observation ohne `current_version` sichtbar; fehlendes `dpkg-query` oder echte Command-Fehler lassen den Job fehlschlagen.

## Lokales Testen auf macOS

Zum lokalen Testen auf macOS ist Homebrew der richtige erste Package-Manager. Dafuer setzt du:

```bash
UP2DATE_NODE_ID=macbook-alex
UP2DATE_INTERVAL=1m

UP2DATE_COLLECTOR_TYPE=package
UP2DATE_COLLECTOR_PACKAGE_MANAGER=brew
UP2DATE_COLLECTOR_PACKAGE_NAMES=samba

UP2DATE_PUBLISHER_TYPE=mqtt
UP2DATE_PUBLISHER_MQTT_HOST=127.0.0.1
```

Der Collector nutzt dann `brew info --formula --json=v2 <name>`. Die Observation sieht dabei gleich aus wie bei `dpkg`, nur mit:

- `artifact_ref = brew:samba`
- `current_version_source = brew info --json=v2`
- `attributes.package_manager = brew`

Wenn du nicht nur den Ist-Stand, sondern auch `latest_version` und `check_status=current|outdated` sehen willst, setze fuer Homebrew zusaetzlich:

```bash
UP2DATE_RESOLVER_TYPE=brew_formula
```

Falls `samba` auf deinem Mac nicht installiert ist, nimm fuer den ersten Test einfach eine vorhandene Formula wie `go`, `ripgrep` oder `python@3.12`.

Als direkt ausfuehrbares Beispiel liegt dafuer [examples/package-brew-mqtt/run.sh](/Users/alex/Workspace/up2date/examples/package-brew-mqtt/run.sh) bereit:

```bash
./examples/package-brew-mqtt/run.sh
```

Fuer Dauerlauf statt One-Shot:

```bash
./examples/package-brew-mqtt/run.sh --continuous
```

## Hinweise

- Das Label `up2date.ignore=true` ist optional. Der Docker-Collector schliesst sich standardmaessig selbst aus.
- Der Docker-Collector erwartet den Socket immer unter `/var/run/docker.sock` im Container.
- Fuer Podman musst du deshalb den Socket des Container-Hosts auf `/var/run/docker.sock` in den Container mounten.
- Auf macOS mit Podman Machine ist `${HOME}/.local/share/containers/podman/machine/podman.sock` oft nicht der richtige Socket fuer den Container. Nutze stattdessen typischerweise den Socket innerhalb der Podman-VM, z. B. `/run/user/1000/podman/podman.sock` oder `/run/podman/podman.sock`.
- Auf SELinux-Systemen kann bei Podman zusaetzlich `security_opt: [label=disable]` noetig sein.
- Aktuell werden nur Docker-Hub-Images bewertet. Andere Registries landen derzeit als `unsupported`.
- Der `package`-Collector unterstuetzt in der ersten Ausbaustufe `dpkg-query` und Homebrew `brew info --formula --json=v2`.
- Published werden nur die einzelnen Feldwerte pro Service.
- `check_status` traegt das Resolver-Ergebnis wie `current`, `outdated`, `unsupported` oder `error`.
- `artifact_name` ist ein vom Collector gelieferter, kurzer Anzeigename des Deployment-Artefakts, also z. B. `nginx`.
- Der Resolver nutzt intern `artifact_ref`, damit `artifact_name` transport- und collectorfreundlich bleiben kann.
- Alte retained Topics aus frueheren Versionen werden nicht automatisch migriert oder geloescht.
- Fuer einen One-Shot-Run kannst du den Container mit `-once` starten.
