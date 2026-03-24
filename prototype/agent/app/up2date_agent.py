from __future__ import annotations

import dataclasses
import datetime as dt
import http.client
import json
import logging
import os
import socket
import sys
import time
import uuid
from typing import Any


LOG = logging.getLogger("up2date-agent")
DEFAULT_TOPIC_PREFIX = "up2date/nodes"
DEFAULT_EXCLUDE_LABEL_SELECTORS = (
    "up2date.ignore=true",
    "com.up2date.ignore=true",
)
VERSION_LABEL_KEYS = (
    "org.opencontainers.image.version",
    "org.label-schema.version",
)


def utc_now() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


def isoformat_z(value: dt.datetime) -> str:
    return value.astimezone(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def env_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def parse_csv_env(name: str, default: tuple[str, ...]) -> tuple[str, ...]:
    raw = os.getenv(name)
    if raw is None:
        return default
    return tuple(part.strip() for part in raw.split(",") if part.strip())


def parse_image_reference(image: str) -> tuple[str, str | None]:
    if not image:
        return "", None
    if "@" in image:
        return image.split("@", 1)[0], None

    last_slash = image.rfind("/")
    last_colon = image.rfind(":")
    if last_colon > last_slash:
        return image[:last_colon], image[last_colon + 1 :]
    return image, None


def version_from_labels(labels: dict[str, str]) -> tuple[str | None, str | None]:
    for key in VERSION_LABEL_KEYS:
        value = labels.get(key)
        if value:
            return value, key
    return None, None


def matches_label_selector(labels: dict[str, str], selector: str) -> bool:
    if "=" in selector:
        key, expected = selector.split("=", 1)
        return labels.get(key) == expected
    return selector in labels


def should_exclude_container(
    container: dict[str, Any],
    *,
    self_container_prefix: str | None,
    exclude_label_selectors: tuple[str, ...],
) -> bool:
    container_id = container.get("Id") or ""
    if self_container_prefix and container_id.startswith(self_container_prefix):
        return True

    labels = container.get("Labels") or {}
    return any(matches_label_selector(labels, selector) for selector in exclude_label_selectors)


class UnixSocketHTTPConnection(http.client.HTTPConnection):
    def __init__(self, unix_socket_path: str, timeout: float = 10.0):
        super().__init__("localhost", timeout=timeout)
        self.unix_socket_path = unix_socket_path

    def connect(self) -> None:
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.settimeout(self.timeout)
        sock.connect(self.unix_socket_path)
        self.sock = sock


@dataclasses.dataclass(slots=True)
class Settings:
    node_id: str
    mqtt_host: str | None
    node_name: str | None = None
    mqtt_port: int = 1883
    mqtt_username: str | None = None
    mqtt_password: str | None = None
    mqtt_topic_prefix: str = DEFAULT_TOPIC_PREFIX
    interval_seconds: int = 60
    docker_socket: str = "/var/run/docker.sock"
    include_stopped: bool = True
    retain_messages: bool = True
    stdout_only: bool = False
    one_shot: bool = False
    docker_fixture_path: str | None = None
    exclude_self: bool = True
    exclude_label_selectors: tuple[str, ...] = DEFAULT_EXCLUDE_LABEL_SELECTORS

    @property
    def self_container_prefix(self) -> str | None:
        if not self.exclude_self:
            return None
        hostname = os.getenv("HOSTNAME", "").strip()
        return hostname or None

    @property
    def snapshot_topic(self) -> str:
        return f"{self.mqtt_topic_prefix}/{self.node_id}/snapshot"

    @property
    def status_topic(self) -> str:
        return f"{self.mqtt_topic_prefix}/{self.node_id}/status"

    @classmethod
    def from_env(cls) -> "Settings":
        node_id = os.getenv("UP2DATE_NODE_ID") or socket.gethostname()
        mqtt_host = os.getenv("UP2DATE_MQTT_HOST")
        interval_raw = os.getenv("UP2DATE_INTERVAL_SECONDS", "60")
        port_raw = os.getenv("UP2DATE_MQTT_PORT", "1883")

        return cls(
            node_id=node_id,
            mqtt_host=mqtt_host,
            node_name=os.getenv("UP2DATE_NODE_NAME"),
            mqtt_port=int(port_raw),
            mqtt_username=os.getenv("UP2DATE_MQTT_USERNAME"),
            mqtt_password=os.getenv("UP2DATE_MQTT_PASSWORD"),
            mqtt_topic_prefix=os.getenv("UP2DATE_MQTT_TOPIC_PREFIX", DEFAULT_TOPIC_PREFIX).rstrip("/"),
            interval_seconds=max(1, int(interval_raw)),
            docker_socket=os.getenv("UP2DATE_DOCKER_SOCKET", "/var/run/docker.sock"),
            include_stopped=env_bool("UP2DATE_INCLUDE_STOPPED", True),
            retain_messages=env_bool("UP2DATE_RETAIN_MESSAGES", True),
            stdout_only=env_bool("UP2DATE_STDOUT_ONLY", False),
            one_shot=env_bool("UP2DATE_ONE_SHOT", False),
            docker_fixture_path=os.getenv("UP2DATE_DOCKER_FIXTURE_PATH"),
            exclude_self=env_bool("UP2DATE_EXCLUDE_SELF", True),
            exclude_label_selectors=parse_csv_env("UP2DATE_EXCLUDE_LABELS", DEFAULT_EXCLUDE_LABEL_SELECTORS),
        )


class DockerCollector:
    def __init__(self, socket_path: str, fixture_path: str | None = None):
        self.socket_path = socket_path
        self.fixture_path = fixture_path

    def list_containers(self, include_stopped: bool) -> list[dict[str, Any]]:
        if self.fixture_path:
            with open(self.fixture_path, "r", encoding="utf-8") as handle:
                return json.load(handle)

        conn = UnixSocketHTTPConnection(self.socket_path, timeout=10.0)
        try:
            all_value = "1" if include_stopped else "0"
            try:
                conn.request("GET", f"/containers/json?all={all_value}")
            except PermissionError as exc:
                raise RuntimeError(self._permission_denied_message()) from exc
            response = conn.getresponse()
            body = response.read()
        finally:
            conn.close()

        if response.status >= 400:
            raise RuntimeError(f"Docker API request failed with {response.status}: {body.decode('utf-8', errors='replace')}")

        return json.loads(body.decode("utf-8"))

    def _permission_denied_message(self) -> str:
        return (
            f"Permission denied while opening container socket {self.socket_path}. "
            "If you are using rootless Podman, mount your user socket into the container via "
            "UP2DATE_HOST_SOCKET_PATH=${XDG_RUNTIME_DIR}/podman/podman.sock. "
            "On SELinux systems, security_opt: [label=disable] may also be required. "
            "If access is granted through supplementary groups, Podman-only keep-groups handling may help, "
            "but some compose providers treat that value as a literal group name."
        )


class MQTTError(RuntimeError):
    pass


class MQTTClient:
    def __init__(
        self,
        host: str,
        port: int,
        client_id: str,
        username: str | None = None,
        password: str | None = None,
        timeout: float = 10.0,
    ):
        self.host = host
        self.port = port
        self.client_id = client_id
        self.username = username
        self.password = password
        self.timeout = timeout
        self.sock: socket.socket | None = None

    def __enter__(self) -> "MQTTClient":
        self.connect()
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        self.disconnect()

    def connect(self) -> None:
        self.sock = socket.create_connection((self.host, self.port), timeout=self.timeout)
        flags = 0x02
        payload = bytearray()
        payload.extend(self._encode_string(self.client_id))

        if self.username is not None:
            flags |= 0x80
            payload.extend(self._encode_string(self.username))
        if self.password is not None:
            flags |= 0x40
            payload.extend(self._encode_string(self.password))

        variable_header = bytearray()
        variable_header.extend(self._encode_string("MQTT"))
        variable_header.append(0x04)
        variable_header.append(flags)
        variable_header.extend((30).to_bytes(2, "big"))

        packet_body = bytes(variable_header + payload)
        packet = bytes([0x10]) + self._encode_remaining_length(len(packet_body)) + packet_body
        self.sock.sendall(packet)

        packet_type, body = self._read_packet()
        if packet_type != 0x20 or len(body) != 2:
            raise MQTTError("Broker returned an unexpected CONNACK packet")
        if body[1] != 0:
            raise MQTTError(f"Broker rejected connection with code {body[1]}")

    def publish(self, topic: str, payload: bytes, retain: bool = False) -> None:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")

        variable_header = self._encode_string(topic)
        fixed_header = 0x30 | (0x01 if retain else 0x00)
        packet_body = variable_header + payload
        packet = bytes([fixed_header]) + self._encode_remaining_length(len(packet_body)) + packet_body
        self.sock.sendall(packet)

    def disconnect(self) -> None:
        if self.sock is None:
            return
        try:
            self.sock.sendall(b"\xe0\x00")
        except OSError:
            pass
        try:
            self.sock.close()
        finally:
            self.sock = None

    def _read_packet(self) -> tuple[int, bytes]:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")

        fixed_header = self._read_exact(1)
        remaining_length = 0
        multiplier = 1
        while True:
            encoded_byte = self._read_exact(1)[0]
            remaining_length += (encoded_byte & 0x7F) * multiplier
            if not encoded_byte & 0x80:
                break
            multiplier *= 128
            if multiplier > 128 * 128 * 128:
                raise MQTTError("Malformed MQTT remaining length")
        return fixed_header[0] & 0xF0, self._read_exact(remaining_length)

    def _read_exact(self, size: int) -> bytes:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")

        data = bytearray()
        while len(data) < size:
            chunk = self.sock.recv(size - len(data))
            if not chunk:
                raise MQTTError("Broker closed the connection")
            data.extend(chunk)
        return bytes(data)

    @staticmethod
    def _encode_string(value: str) -> bytes:
        raw = value.encode("utf-8")
        return len(raw).to_bytes(2, "big") + raw

    @staticmethod
    def _encode_remaining_length(value: int) -> bytes:
        encoded = bytearray()
        while True:
            digit = value % 128
            value //= 128
            if value > 0:
                digit |= 0x80
            encoded.append(digit)
            if value == 0:
                break
        return bytes(encoded)


def build_service(container: dict[str, Any]) -> dict[str, Any]:
    labels = container.get("Labels") or {}
    container_name = first_container_name(container)
    image = container.get("Image", "")
    image_name, image_tag = parse_image_reference(image)

    detected_version, version_label_key = version_from_labels(labels)
    detected_version_source = "container_label"
    if not detected_version:
        detected_version = image_tag or "unknown"
        detected_version_source = "image_tag" if image_tag else "unknown"

    project_name = labels.get("com.docker.compose.project")
    service_name = labels.get("com.docker.compose.service") or container_name

    service = {
        "container_id": (container.get("Id") or "")[:12],
        "container_name": container_name,
        "service_name": service_name,
        "image": image,
        "detected_version": detected_version,
        "detected_version_source": detected_version_source,
        "state": container.get("State") or "unknown",
        "running": (container.get("State") == "running"),
        "status": container.get("Status") or "",
        "observed_via": "docker_engine",
    }

    if project_name:
        service["project_name"] = project_name
    if image_name:
        service["image_name"] = image_name
    if image_tag:
        service["image_tag"] = image_tag
    if version_label_key:
        service["version_label_key"] = version_label_key

    return service


def first_container_name(container: dict[str, Any]) -> str:
    names = container.get("Names") or []
    if not names:
        return (container.get("Id") or "")[:12]
    return names[0].lstrip("/")


def build_snapshot(
    node_id: str,
    containers: list[dict[str, Any]],
    *,
    observed_at: dt.datetime | None = None,
    node_name: str | None = None,
    self_container_prefix: str | None = None,
    exclude_label_selectors: tuple[str, ...] = DEFAULT_EXCLUDE_LABEL_SELECTORS,
) -> dict[str, Any]:
    observed_at = observed_at or utc_now()
    services: list[dict[str, Any]] = []

    for container in containers:
        if should_exclude_container(
            container,
            self_container_prefix=self_container_prefix,
            exclude_label_selectors=exclude_label_selectors,
        ):
            continue
        services.append(build_service(container))

    services.sort(key=lambda item: (item.get("project_name", ""), item["service_name"], item["container_name"]))

    snapshot = {
        "schema_version": 1,
        "kind": "docker_node_snapshot",
        "agent_id": node_id,
    }
    if node_name:
        snapshot["node_name"] = node_name
    snapshot["observed_at"] = isoformat_z(observed_at)
    snapshot["services"] = services
    return snapshot


def build_status(snapshot: dict[str, Any]) -> dict[str, Any]:
    services = snapshot["services"]
    return {
        "schema_version": 1,
        "kind": "agent_status",
        "agent_id": snapshot["agent_id"],
        "observed_at": snapshot["observed_at"],
        "service_count": len(services),
        "running_service_count": sum(1 for service in services if service["running"]),
    }


def publish_cycle(settings: Settings, snapshot: dict[str, Any], status: dict[str, Any]) -> None:
    if settings.stdout_only:
        print(json.dumps({"topic": settings.status_topic, "payload": status}, indent=2))
        print(json.dumps({"topic": settings.snapshot_topic, "payload": snapshot}, indent=2))
        return

    if not settings.mqtt_host:
        raise RuntimeError("UP2DATE_MQTT_HOST must be set unless UP2DATE_STDOUT_ONLY=true")

    client_id = f"up2date-{settings.node_id}-{uuid.uuid4().hex[:8]}"
    with MQTTClient(
        host=settings.mqtt_host,
        port=settings.mqtt_port,
        client_id=client_id,
        username=settings.mqtt_username,
        password=settings.mqtt_password,
    ) as mqtt:
        mqtt.publish(settings.status_topic, json.dumps(status, separators=(",", ":")).encode("utf-8"), retain=settings.retain_messages)
        mqtt.publish(settings.snapshot_topic, json.dumps(snapshot, separators=(",", ":")).encode("utf-8"), retain=settings.retain_messages)


def configure_logging() -> None:
    level_name = os.getenv("UP2DATE_LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    logging.basicConfig(level=level, format="%(asctime)s %(levelname)s %(message)s")


def run() -> int:
    configure_logging()
    settings = Settings.from_env()
    collector = DockerCollector(settings.docker_socket, settings.docker_fixture_path)

    LOG.info(
        "starting up2date-agent for node=%s node_name=%s interval=%ss socket=%s",
        settings.node_id,
        settings.node_name or "-",
        settings.interval_seconds,
        settings.docker_socket,
    )

    while True:
        try:
            containers = collector.list_containers(include_stopped=settings.include_stopped)
            snapshot = build_snapshot(
                settings.node_id,
                containers,
                node_name=settings.node_name,
                self_container_prefix=settings.self_container_prefix,
                exclude_label_selectors=settings.exclude_label_selectors,
            )
            status = build_status(snapshot)
            publish_cycle(settings, snapshot, status)
            LOG.info("published snapshot for node=%s services=%s running=%s", settings.node_id, status["service_count"], status["running_service_count"])
            if settings.one_shot:
                return 0
        except KeyboardInterrupt:
            LOG.info("received interrupt, stopping agent")
            return 0
        except Exception as exc:
            LOG.exception("publish cycle failed: %s", exc)
            if settings.one_shot:
                return 1

        time.sleep(settings.interval_seconds)


def main() -> int:
    try:
        return run()
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
