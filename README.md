# up2date

`up2date` liest aktuell Docker-Container von `docker.sock` oder lokal installierte OS-Pakete, prueft verfuegbare Versionen ueber einen Resolver und published das Ergebnis per MQTT.

Der Collector liefert die aktuellen Fakten eines Workloads, inklusive `artifact_name` und `artifact_ref`. Der Resolver arbeitet auf dem Referenzfeld und reichert nur um Upstream-Informationen an. Der Publisher published nur die bereits vorbereiteten Felder.

Die Konfiguration kann ueber Umgebungsvariablen oder ueber eine einfache YAML-/JSON-Datei passieren.

Beim Docker-Collector ist der Standard weiter der lokale Unix-Socket `/var/run/docker.sock`. Optional kannst du stattdessen aber auch einen Docker-API-Endpoint wie `tcp://dockerproxy:2375` angeben, z. B. wenn du einen eingeschraenkten Socket-Proxy dazwischen setzen willst.

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
  ...
  
  up2date:
    build: .
    restart: unless-stopped
    environment:
      UP2DATE_NODE_ID: docker-host-01
      UP2DATE_INTERVAL: 1m

      UP2DATE_COLLECTOR_TYPE: docker

      UP2DATE_PUBLISHER_TYPE: mqtt
      UP2DATE_PUBLISHER_MQTT_HOST: MQTT_HOST
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

Starten:

```bash
docker compose up --build -d
```

Mit einem Socket-Proxy statt direktem Socket-Mount sieht der Collector-Teil z. B. so aus:

```yaml
services:
  dockerproxy:
    image: tecnativa/docker-socket-proxy:latest
    restart: unless-stopped
    environment:
      CONTAINERS: 1
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro

  up2date:
    build: .
    restart: unless-stopped
    depends_on:
      - dockerproxy
    environment:
      UP2DATE_NODE_ID: docker-host-01
      UP2DATE_INTERVAL: 1m
      UP2DATE_COLLECTOR_TYPE: docker
      UP2DATE_COLLECTOR_DOCKER_ENDPOINT: tcp://dockerproxy:2375
      UP2DATE_PUBLISHER_TYPE: mqtt
      UP2DATE_PUBLISHER_MQTT_HOST: MQTT_HOST
```

Dabei bekommt nur der Proxy Zugriff auf `/var/run/docker.sock`; `up2date` spricht nur noch mit dem eingeschraenkten HTTP-Endpoint des Proxys.

## Binary mit Config-Datei

Fuer LXC-, VM- oder Bare-Metal-Installationen ist eine Datei meist angenehmer als viele einzelne Umgebungsvariablen.

`up2date` unterstuetzt dafuer eine einfache YAML- oder JSON-Datei. Ein direkt nutzbares Beispiel fuer den bestehenden Package-Demo-Flow liegt unter [examples/package-brew-mqtt/config.yml](/Users/alex/Workspace/up2date/examples/package-brew-mqtt/config.yml).

```yaml
node_id: my-device
interval: 1m
job_name: package

collector:
  type: package
  package:
    manager: dpkg
    names:
      - samba

publisher:
  mqtt:
    host: MQTT_HOST
```

Starten:

```bash
./up2date -config /etc/up2date/up2date.yaml
```

Oder per Pfad-Variable:

```bash
UP2DATE_CONFIG_FILE=/etc/up2date/up2date.yaml ./up2date
```

Der Resolver ist in Datei-Configs absichtlich nicht konfigurierbar. Er wird automatisch aus Collector und Paketmanager abgeleitet:

- `docker` -> `docker`
- `package` + `brew` -> `brew_formula`
- `package` + `dpkg` -> `none`

Fuer den Docker-Collector kannst du in Datei-Configs optional auch einen Endpoint setzen:

```yaml
collector:
  type: docker
  docker:
    endpoint: tcp://dockerproxy:2375
```

## Wichtige Variablen

Allgemein:
- `UP2DATE_NODE_ID`
- `UP2DATE_INTERVAL`
- `UP2DATE_CONFIG_FILE` optional, Alternative zu `-config /pfad/zur/datei`

Collector:
- `UP2DATE_COLLECTOR_TYPE`: `docker` oder `package`

Resolver:
- Resolver werden standardmaessig automatisch abgeleitet:
  - `docker` -> `docker`
  - `package` + `brew` -> `brew_formula`
  - `package` + `dpkg` -> `none`
- `UP2DATE_RESOLVER_TYPE` bleibt optional als explizite Env-Uebersteuerung fuer fortgeschrittene Faelle
- Fuer Docker akzeptiert `UP2DATE_RESOLVER_TYPE` aus Rueckwaertskompatibilitaet sowohl `docker` als auch das alte Alias `docker_hub`

Collector `docker`:
- `UP2DATE_COLLECTOR_DOCKER_ENDPOINT` optional, z. B. `unix:///var/run/docker.sock` oder `tcp://dockerproxy:2375`
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

Fuer `dpkg` bleibt der automatisch abgeleitete Resolver absichtlich `none`. Damit trennen wir die Frage "was ist installiert?" sauber von "was waere die passende neuere Version?".

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
UP2DATE_NODE_ID=my-device
UP2DATE_INTERVAL=1m

UP2DATE_COLLECTOR_TYPE=package
UP2DATE_COLLECTOR_PACKAGE_MANAGER=MQTT_HOST
UP2DATE_COLLECTOR_PACKAGE_NAMES=samba

UP2DATE_PUBLISHER_TYPE=mqtt
UP2DATE_PUBLISHER_MQTT_HOST=127.0.0.1
```

Der Collector nutzt dann `brew info --formula --json=v2 <name>`. Die Observation sieht dabei gleich aus wie bei `dpkg`, nur mit:

- `artifact_ref = brew:samba`
- `current_version_source = brew info --json=v2`
- `attributes.package_manager = brew`

Der Resolver wird fuer Homebrew automatisch als `brew_formula` abgeleitet. Damit bekommst du ohne Zusatzkonfiguration auch `latest_version` und `check_status=current|outdated`.

Falls `samba` auf deinem Mac nicht installiert ist, nimm fuer den ersten Test einfach eine vorhandene Formula wie `go`, `ripgrep` oder `python@3.12`.

Als direkt ausfuehrbares Beispiel liegt dafuer [examples/package-brew-mqtt/run.sh](/Users/alex/Workspace/up2date/examples/package-brew-mqtt/run.sh) bereit:

```bash
./examples/package-brew-mqtt/run.sh
```

Das statische Beispiel-Config-File dazu liegt unter [examples/package-brew-mqtt/config.yml](/Users/alex/Workspace/up2date/examples/package-brew-mqtt/config.yml). `run.sh` startet nur den lokalen MQTT-Broker, baut `up2date` als Binary und ruft dieses Binary dann mit `-config` auf.

Fuer Dauerlauf statt One-Shot:

```bash
./examples/package-brew-mqtt/run.sh --continuous
```

## Hinweise

- Das Label `up2date.ignore=true` ist optional. Der Docker-Collector schliesst sich standardmaessig selbst aus.
- Ohne `UP2DATE_COLLECTOR_DOCKER_ENDPOINT` nutzt der Docker-Collector den Unix-Socket `/var/run/docker.sock`.
- Fuer Podman kannst du entweder wie bisher den Socket des Container-Hosts auf `/var/run/docker.sock` in den Container mounten oder direkt einen Unix-Endpoint wie `unix:///run/podman/podman.sock` konfigurieren.
- Fuer einen lokalen Socket-Proxy kannst du statt des direkten Socket-Mounts einen TCP-Endpoint wie `tcp://dockerproxy:2375` setzen.
- Wenn du [Tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) nutzt, braucht `up2date` fuer den aktuellen Collector mindestens Zugriff auf `GET /containers/json`; bei diesem Proxy heisst das in der Praxis `CONTAINERS=1`. Den Proxy-Port solltest du nur im internen Docker-Netz veroeffentlichen, nicht ins Host-Netz.
- Auf macOS mit Podman Machine ist `${HOME}/.local/share/containers/podman/machine/podman.sock` oft nicht der richtige Socket fuer den Container. Nutze stattdessen typischerweise den Socket innerhalb der Podman-VM, z. B. `/run/user/1000/podman/podman.sock` oder `/run/podman/podman.sock`.
- Auf SELinux-Systemen kann bei Podman zusaetzlich `security_opt: [label=disable]` noetig sein.
- Der Docker-Resolver entscheidet anhand von `artifact_ref`, welche Registry verwendet wird. Aktuell unterstuetzt er Docker Hub und GHCR. Andere Registries landen derzeit als `unsupported`.
- Fuer parsebare numerische Docker-Tags vergleicht der Resolver aktuell Kandidaten mit gleicher Segmentzahl und gleichem Suffix. Ein Tag wie `17` kann dadurch auch auf `18` als neueres Release zeigen, waehrend `17.1-alpine` weiter nur mit anderen `x.y-alpine`-Tags verglichen wird.
- Der `package`-Collector unterstuetzt in der ersten Ausbaustufe `dpkg-query` und Homebrew `brew info --formula --json=v2`.
- Datei-Configs unterstuetzen den einfachen YAML-/JSON-Stil aus den Beispielen, also verschachtelte Mappings und String-Listen.
- Published werden nur die einzelnen Feldwerte pro Service.
- `check_status` traegt das Resolver-Ergebnis wie `current`, `outdated`, `unsupported` oder `error`.
- `artifact_name` ist ein vom Collector gelieferter, kurzer Anzeigename des Deployment-Artefakts, also z. B. `nginx`.
- Fuer GHCR liefert der Docker-Resolver aktuell `latest_version`, aber noch keinen `latest_version_url`.
- Der Resolver nutzt intern `artifact_ref`, damit `artifact_name` transport- und collectorfreundlich bleiben kann.
- Alte retained Topics aus frueheren Versionen werden nicht automatisch migriert oder geloescht.
- Fuer einen One-Shot-Run kannst du den Container mit `-once` starten.
