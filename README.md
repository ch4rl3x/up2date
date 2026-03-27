# up2date

`up2date` liest aktuell Docker-Container von `docker.sock`, prueft verfuegbare Tags auf Docker Hub und published das Ergebnis per MQTT.

Der Collector liefert die aktuellen Fakten eines Workloads, inklusive `artifact_name` und `artifact_ref`. Der Resolver arbeitet auf dem Referenzfeld und reichert nur um Upstream-Informationen an. Der Publisher published nur die bereits vorbereiteten Felder.

Die Konfiguration passiert ausschliesslich ueber Umgebungsvariablen.

## Nutzung mit Docker Compose

Zum direkten Testen kannst du das Beispiel unter [examples/compose.yml](/Users/alex/Workspace/up2date/examples/compose.yml) starten:

```bash
docker compose -f examples/compose.yml up --build -d
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
- `UP2DATE_COLLECTOR_TYPE`: aktuell nur `docker`

Resolver:
- `UP2DATE_RESOLVER_TYPE`: aktuell `docker_hub`, fuer den Docker-Collector standardmaessig implizit gesetzt

Collector `docker`:
- `UP2DATE_COLLECTOR_DOCKER_INCLUDE_STOPPED`
- `UP2DATE_COLLECTOR_DOCKER_EXCLUDE_SELF`
- `UP2DATE_COLLECTOR_DOCKER_EXCLUDE_LABELS`

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

## Hinweise

- Das Label `up2date.ignore=true` ist optional. Der Docker-Collector schliesst sich standardmaessig selbst aus.
- Der Docker-Collector erwartet den Socket immer unter `/var/run/docker.sock` im Container.
- Fuer Podman musst du deshalb den Socket des Container-Hosts auf `/var/run/docker.sock` in den Container mounten.
- Auf macOS mit Podman Machine ist `${HOME}/.local/share/containers/podman/machine/podman.sock` oft nicht der richtige Socket fuer den Container. Nutze stattdessen typischerweise den Socket innerhalb der Podman-VM, z. B. `/run/user/1000/podman/podman.sock` oder `/run/podman/podman.sock`.
- Auf SELinux-Systemen kann bei Podman zusaetzlich `security_opt: [label=disable]` noetig sein.
- Aktuell werden nur Docker-Hub-Images bewertet. Andere Registries landen derzeit als `unsupported`.
- Published werden nur die einzelnen Feldwerte pro Service.
- `check_status` traegt das Resolver-Ergebnis wie `current`, `outdated`, `unsupported` oder `error`.
- `artifact_name` ist ein vom Collector gelieferter, kurzer Anzeigename des Deployment-Artefakts, also z. B. `nginx`.
- Der Resolver nutzt intern `artifact_ref`, damit `artifact_name` transport- und collectorfreundlich bleiben kann.
- Alte retained Topics aus frueheren Versionen werden nicht automatisch migriert oder geloescht.
- Fuer einen One-Shot-Run kannst du den Container mit `-once` starten.
