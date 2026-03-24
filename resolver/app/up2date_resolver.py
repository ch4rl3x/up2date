from __future__ import annotations

import datetime as dt
import json
import logging
import os
import re
import socket
import ssl
import sys
import time
import uuid
from dataclasses import dataclass
from http.client import HTTPConnection, HTTPSConnection
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlparse, urlunparse


LOG = logging.getLogger("up2date-resolver")
DEFAULT_TOPIC_PREFIX = "up2date/nodes"
AUTH_BEARER_RE = re.compile(r'Bearer\s+(?P<params>.+)', re.IGNORECASE)
AUTH_PARAM_RE = re.compile(r'(\w+)="([^"]*)"')
VERSION_RE = re.compile(r"^v?(?P<numbers>\d+(?:\.\d+)*)(?P<suffix>[-._][A-Za-z0-9][A-Za-z0-9._-]*)?$")
TOPIC_SEGMENT_RE = re.compile(r"[^A-Za-z0-9._-]+")


def utc_now() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


def isoformat_z(value: dt.datetime) -> str:
    return value.astimezone(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def env_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def parse_csv_env(name: str, default: tuple[str, ...] = ()) -> tuple[str, ...]:
    raw = os.getenv(name)
    if raw is None:
        return default
    return tuple(part.strip() for part in raw.split(",") if part.strip())


def sanitize_topic_segment(value: str) -> str:
    sanitized = TOPIC_SEGMENT_RE.sub("_", value.strip())
    return sanitized or "unknown"


@dataclass(slots=True)
class Settings:
    mqtt_host: str | None
    mqtt_port: int = 1883
    mqtt_username: str | None = None
    mqtt_password: str | None = None
    mqtt_topic_prefix: str = DEFAULT_TOPIC_PREFIX
    retain_messages: bool = True
    stdout_only: bool = False
    one_shot: bool = False
    snapshot_fixture_path: str | None = None
    registry_fixture_path: str | None = None
    registry_timeout_seconds: int = 20
    insecure_registries: tuple[str, ...] = ()
    read_timeout_seconds: int = 30

    @property
    def snapshot_topic_filter(self) -> str:
        return f"{self.mqtt_topic_prefix}/+/snapshot"

    def check_topic(self, node_id: str, service_name: str) -> str:
        return f"{self.mqtt_topic_prefix}/{node_id}/checks/{sanitize_topic_segment(service_name)}"

    @classmethod
    def from_env(cls) -> "Settings":
        port_raw = os.getenv("UP2DATE_MQTT_PORT", "1883")
        timeout_raw = os.getenv("UP2DATE_REGISTRY_TIMEOUT_SECONDS", "20")
        read_timeout_raw = os.getenv("UP2DATE_MQTT_READ_TIMEOUT_SECONDS", "30")

        return cls(
            mqtt_host=os.getenv("UP2DATE_MQTT_HOST"),
            mqtt_port=int(port_raw),
            mqtt_username=os.getenv("UP2DATE_MQTT_USERNAME"),
            mqtt_password=os.getenv("UP2DATE_MQTT_PASSWORD"),
            mqtt_topic_prefix=os.getenv("UP2DATE_MQTT_TOPIC_PREFIX", DEFAULT_TOPIC_PREFIX).rstrip("/"),
            retain_messages=env_bool("UP2DATE_RETAIN_MESSAGES", True),
            stdout_only=env_bool("UP2DATE_STDOUT_ONLY", False),
            one_shot=env_bool("UP2DATE_ONE_SHOT", False),
            snapshot_fixture_path=os.getenv("UP2DATE_SNAPSHOT_FIXTURE_PATH"),
            registry_fixture_path=os.getenv("UP2DATE_REGISTRY_FIXTURE_PATH"),
            registry_timeout_seconds=max(1, int(timeout_raw)),
            insecure_registries=parse_csv_env("UP2DATE_INSECURE_REGISTRIES"),
            read_timeout_seconds=max(1, int(read_timeout_raw)),
        )


class MQTTError(RuntimeError):
    pass


class MQTTClient:
    def __init__(
        self,
        host: str,
        port: int,
        client_id: str,
        *,
        username: str | None = None,
        password: str | None = None,
        connect_timeout: float = 10.0,
        read_timeout: float = 30.0,
        keepalive_seconds: int = 120,
    ):
        self.host = host
        self.port = port
        self.client_id = client_id
        self.username = username
        self.password = password
        self.connect_timeout = connect_timeout
        self.read_timeout = read_timeout
        self.keepalive_seconds = keepalive_seconds
        self.sock: socket.socket | None = None
        self._packet_id = 1

    def __enter__(self) -> "MQTTClient":
        self.connect()
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        self.disconnect()

    def connect(self) -> None:
        self.sock = socket.create_connection((self.host, self.port), timeout=self.connect_timeout)
        self.sock.settimeout(self.read_timeout)

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
        variable_header.extend(self.keepalive_seconds.to_bytes(2, "big"))

        packet_body = bytes(variable_header + payload)
        packet = bytes([0x10]) + self._encode_remaining_length(len(packet_body)) + packet_body
        self.sock.sendall(packet)

        packet_type, _flags, body = self._read_packet()
        if packet_type != 2 or len(body) != 2:
            raise MQTTError("Broker returned an unexpected CONNACK packet")
        if body[1] != 0:
            raise MQTTError(f"Broker rejected connection with code {body[1]}")

    def subscribe(self, topic_filter: str) -> None:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")

        packet_id = self._next_packet_id()
        variable_header = packet_id.to_bytes(2, "big")
        payload = self._encode_string(topic_filter) + b"\x00"
        packet_body = variable_header + payload
        packet = bytes([0x82]) + self._encode_remaining_length(len(packet_body)) + packet_body
        self.sock.sendall(packet)

        packet_type, _flags, body = self._read_packet()
        if packet_type != 9:
            raise MQTTError("Broker returned an unexpected packet instead of SUBACK")
        if body[:2] != packet_id.to_bytes(2, "big"):
            raise MQTTError("Broker returned a mismatched SUBACK packet id")
        if len(body) < 3 or body[2] == 0x80:
            raise MQTTError("Broker rejected the subscription")

    def publish(self, topic: str, payload: bytes, retain: bool = False) -> None:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")

        variable_header = self._encode_string(topic)
        fixed_header = 0x30 | (0x01 if retain else 0x00)
        packet_body = variable_header + payload
        packet = bytes([fixed_header]) + self._encode_remaining_length(len(packet_body)) + packet_body
        self.sock.sendall(packet)

    def read_publish(self) -> tuple[str, bytes]:
        while True:
            try:
                packet_type, flags, body = self._read_packet()
            except socket.timeout:
                self.ping()
                continue

            if packet_type == 3:
                return self._decode_publish(flags, body)
            if packet_type == 13:
                continue

    def ping(self) -> None:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")
        self.sock.sendall(b"\xc0\x00")

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

    def _next_packet_id(self) -> int:
        packet_id = self._packet_id
        self._packet_id += 1
        if self._packet_id > 0xFFFF:
            self._packet_id = 1
        return packet_id

    def _read_packet(self) -> tuple[int, int, bytes]:
        if self.sock is None:
            raise MQTTError("MQTT client is not connected")

        fixed_header = self._read_exact(1)[0]
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
        return fixed_header >> 4, fixed_header & 0x0F, self._read_exact(remaining_length)

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

    def _decode_publish(self, flags: int, body: bytes) -> tuple[str, bytes]:
        if len(body) < 2:
            raise MQTTError("Malformed PUBLISH packet")

        topic_length = int.from_bytes(body[0:2], "big")
        if len(body) < 2 + topic_length:
            raise MQTTError("Malformed PUBLISH packet topic length")
        topic = body[2 : 2 + topic_length].decode("utf-8")
        payload_index = 2 + topic_length

        qos = (flags >> 1) & 0x03
        if qos > 0:
            payload_index += 2
        return topic, body[payload_index:]

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


@dataclass(frozen=True, slots=True)
class RegistryReference:
    registry_host: str
    repository: str
    display_name: str


@dataclass(frozen=True, slots=True)
class ParsedVersion:
    raw: str
    numbers: tuple[int, ...]
    suffix: str


@dataclass(frozen=True, slots=True)
class Resolution:
    latest_version: str | None
    latest_version_source: str | None
    status: str
    update_available: bool | None
    reason: str | None
    comparison_track: str | None
    registry: str | None
    repository: str | None


@dataclass(frozen=True, slots=True)
class CheckPublication:
    topic: str
    payload: dict[str, Any]


@dataclass(frozen=True, slots=True)
class HTTPResult:
    status: int
    headers: dict[str, str]
    body: bytes


def parse_registry_reference(image_name: str) -> RegistryReference:
    parts = image_name.split("/")
    if len(parts) > 1 and ("." in parts[0] or ":" in parts[0] or parts[0] == "localhost"):
        registry_host = parts[0]
        repository = "/".join(parts[1:])
    else:
        registry_host = "registry-1.docker.io"
        repository = image_name
        if "/" not in repository:
            repository = f"library/{repository}"

    if registry_host in {"docker.io", "index.docker.io"}:
        registry_host = "registry-1.docker.io"

    return RegistryReference(
        registry_host=registry_host,
        repository=repository,
        display_name=f"{registry_host}/{repository}",
    )


def parse_version(value: str) -> ParsedVersion | None:
    match = VERSION_RE.match(value)
    if not match:
        return None
    numbers = tuple(int(part) for part in match.group("numbers").split("."))
    suffix = match.group("suffix") or ""
    return ParsedVersion(raw=value, numbers=numbers, suffix=suffix)


def select_current_version(service: dict[str, Any]) -> str | None:
    detected_version = service.get("detected_version")
    image_tag = service.get("image_tag")

    if isinstance(detected_version, str) and parse_version(detected_version):
        return detected_version
    if isinstance(image_tag, str) and parse_version(image_tag):
        return image_tag

    if isinstance(detected_version, str) and detected_version.strip():
        return detected_version
    if isinstance(image_tag, str) and image_tag.strip():
        return image_tag
    return None


def link_next_path(link_header: str | None) -> str | None:
    if not link_header:
        return None
    if ";" not in link_header:
        return None
    target, rest = link_header.split(";", 1)
    if 'rel="next"' not in rest:
        return None
    return target.strip()[1:-1]


def parse_bearer_challenge(header_value: str) -> dict[str, str]:
    match = AUTH_BEARER_RE.match(header_value.strip())
    if not match:
        raise RuntimeError("Unsupported WWW-Authenticate challenge")
    params_raw = match.group("params")
    return {key: value for key, value in AUTH_PARAM_RE.findall(params_raw)}


def append_query_params(url: str, params: dict[str, str]) -> str:
    parsed = urlparse(url)
    current = dict(parse_qsl(parsed.query))
    current.update({key: value for key, value in params.items() if value})
    return urlunparse(parsed._replace(query=urlencode(current)))


class RegistryClient:
    def __init__(
        self,
        *,
        timeout_seconds: int = 20,
        insecure_registries: tuple[str, ...] = (),
        fixture_tags: dict[str, list[str]] | None = None,
    ) -> None:
        self.timeout_seconds = timeout_seconds
        self.insecure_registries = set(insecure_registries)
        self.fixture_tags = fixture_tags or {}

    def list_tags(self, image_name: str) -> tuple[list[str], RegistryReference]:
        reference = parse_registry_reference(image_name)
        if image_name in self.fixture_tags:
            return sorted(set(self.fixture_tags[image_name])), reference
        tags: list[str] = []
        token: str | None = None
        path = f"/v2/{reference.repository}/tags/list?n=1000"

        while path:
            response = self._request(reference.registry_host, "GET", path, token=token)
            if response.status == 401:
                header_value = response.headers.get("www-authenticate")
                if not header_value:
                    raise RuntimeError(f"Registry {reference.registry_host} requested auth without a challenge header")
                token = self._fetch_bearer_token(reference, header_value)
                continue
            if response.status >= 400:
                body = response.body.decode("utf-8", errors="replace")
                raise RuntimeError(f"Registry {reference.display_name} returned {response.status}: {body}")

            payload = json.loads(response.body.decode("utf-8"))
            tags.extend(payload.get("tags") or [])

            next_path = link_next_path(response.headers.get("link"))
            if next_path:
                parsed_next = urlparse(next_path)
                path = next_path if parsed_next.scheme else next_path
            else:
                path = None

        return sorted(set(tags)), reference

    def _fetch_bearer_token(self, reference: RegistryReference, auth_header: str) -> str:
        challenge = parse_bearer_challenge(auth_header)
        realm = challenge.get("realm")
        if not realm:
            raise RuntimeError("Registry auth challenge is missing a token realm")

        token_url = append_query_params(
            realm,
            {
                "service": challenge.get("service", ""),
                "scope": challenge.get("scope", f"repository:{reference.repository}:pull"),
            },
        )

        parsed = urlparse(token_url)
        path = parsed.path or "/"
        if parsed.query:
            path = f"{path}?{parsed.query}"

        response = self._request(parsed.netloc, "GET", path, token=None, scheme=parsed.scheme or "https")
        if response.status >= 400:
            body = response.body.decode("utf-8", errors="replace")
            raise RuntimeError(f"Token endpoint {parsed.netloc} returned {response.status}: {body}")

        payload = json.loads(response.body.decode("utf-8"))
        token = payload.get("token") or payload.get("access_token")
        if not token:
            raise RuntimeError("Token endpoint did not return a bearer token")
        return token

    def _request(
        self,
        host: str,
        method: str,
        path: str,
        *,
        token: str | None,
        scheme: str | None = None,
    ) -> HTTPResult:
        scheme = scheme or ("http" if host in self.insecure_registries else "https")
        connection_cls = HTTPConnection if scheme == "http" else HTTPSConnection
        kwargs: dict[str, Any] = {"timeout": self.timeout_seconds}
        if connection_cls is HTTPSConnection:
            kwargs["context"] = ssl.create_default_context()
        connection = connection_cls(host, **kwargs)

        headers = {"Accept": "application/json"}
        if token:
            headers["Authorization"] = f"Bearer {token}"

        try:
            connection.request(method, path, headers=headers)
            response = connection.getresponse()
            body = response.read()
            headers_map = {key.lower(): value for key, value in response.getheaders()}
            return HTTPResult(status=response.status, headers=headers_map, body=body)
        finally:
            connection.close()


def determine_resolution(service: dict[str, Any], tags: list[str], reference: RegistryReference) -> Resolution:
    current_version = select_current_version(service) or ""
    image_tag = service.get("image_tag") or ""

    parsed_current = parse_version(current_version)
    parsed_image_tag = parse_version(image_tag) if image_tag else None

    if not parsed_current:
        return Resolution(
            latest_version=None,
            latest_version_source=None,
            status="unsupported",
            update_available=None,
            reason="Current version is not parseable as a numeric release tag",
            comparison_track=None,
            registry=reference.registry_host,
            repository=reference.repository,
        )

    parsed_candidates = [candidate for candidate in (parse_version(tag) for tag in tags) if candidate is not None]
    if not parsed_candidates:
        return Resolution(
            latest_version=None,
            latest_version_source=None,
            status="unknown",
            update_available=None,
            reason="Registry did not return any parseable release tags",
            comparison_track=None,
            registry=reference.registry_host,
            repository=reference.repository,
        )

    if parsed_image_tag and len(parsed_image_tag.numbers) == 1:
        comparison_track = "same_major_same_shape"
        parsed_candidates = [
            candidate
            for candidate in parsed_candidates
            if candidate.numbers[0] == parsed_image_tag.numbers[0]
            and candidate.suffix == parsed_current.suffix
            and len(candidate.numbers) == len(parsed_current.numbers)
        ]
    else:
        comparison_track = "same_suffix_same_shape"
        parsed_candidates = [
            candidate
            for candidate in parsed_candidates
            if candidate.suffix == parsed_current.suffix and len(candidate.numbers) == len(parsed_current.numbers)
        ]

    if not parsed_candidates:
        return Resolution(
            latest_version=None,
            latest_version_source=None,
            status="unknown",
            update_available=None,
            reason="No registry tags matched the current version scheme with high confidence",
            comparison_track=comparison_track,
            registry=reference.registry_host,
            repository=reference.repository,
        )

    latest_candidate = max(parsed_candidates, key=lambda candidate: candidate.numbers)
    update_available = parsed_current.numbers < latest_candidate.numbers
    status = "outdated" if update_available else "current"

    return Resolution(
        latest_version=latest_candidate.raw,
        latest_version_source="docker_registry_tag",
        status=status,
        update_available=update_available,
        reason=None,
        comparison_track=comparison_track,
        registry=reference.registry_host,
        repository=reference.repository,
    )


def build_check(snapshot: dict[str, Any], service: dict[str, Any], resolution: Resolution) -> dict[str, Any]:
    check = {
        "schema_version": 1,
        "kind": "service_check",
        "node_id": snapshot["agent_id"],
        "service_name": service["service_name"],
        "observed_at": isoformat_z(utc_now()),
        "current_version": select_current_version(service),
        "status": resolution.status,
        "update_available": resolution.update_available,
    }

    if snapshot.get("node_name"):
        check["node_name"] = snapshot["node_name"]
    if service.get("image_name"):
        check["image_name"] = service["image_name"]
    if resolution.latest_version is not None:
        check["latest_version"] = resolution.latest_version
    if resolution.reason is not None:
        check["reason"] = resolution.reason

    return check


class ResolverEngine:
    def __init__(self, settings: Settings, registry_client: RegistryClient) -> None:
        self.settings = settings
        self.registry_client = registry_client

    def process_snapshot(self, snapshot: dict[str, Any]) -> list[CheckPublication]:
        node_id = snapshot["agent_id"]
        publications: list[CheckPublication] = []
        registry_cache: dict[str, tuple[list[str], RegistryReference]] = {}

        for service in snapshot.get("services", []):
            check = self._process_service(snapshot, service, registry_cache)
            topic = self.settings.check_topic(node_id, service["service_name"])
            publications.append(CheckPublication(topic=topic, payload=check))

        return publications

    def _process_service(
        self,
        snapshot: dict[str, Any],
        service: dict[str, Any],
        registry_cache: dict[str, tuple[list[str], RegistryReference]],
    ) -> dict[str, Any]:
        image_name = service.get("image_name")
        if not image_name:
            resolution = Resolution(
                latest_version=None,
                latest_version_source=None,
                status="unsupported",
                update_available=None,
                reason="Service is missing image_name",
                comparison_track=None,
                registry=None,
                repository=None,
            )
            return build_check(snapshot, service, resolution)

        current_version = select_current_version(service) or ""
        if not parse_version(current_version):
            resolution = Resolution(
                latest_version=None,
                latest_version_source=None,
                status="unsupported",
                update_available=None,
                reason="Current version is not parseable as a numeric release tag",
                comparison_track=None,
                registry=None,
                repository=None,
            )
            return build_check(snapshot, service, resolution)

        try:
            if image_name in registry_cache:
                tags, reference = registry_cache[image_name]
            else:
                tags, reference = self.registry_client.list_tags(image_name)
                registry_cache[image_name] = (tags, reference)
            resolution = determine_resolution(service, tags, reference)
        except Exception as exc:
            resolution = Resolution(
                latest_version=None,
                latest_version_source=None,
                status="error",
                update_available=None,
                reason=str(exc),
                comparison_track=None,
                registry=None,
                repository=None,
            )

        return build_check(snapshot, service, resolution)


def configure_logging() -> None:
    level_name = os.getenv("UP2DATE_LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    logging.basicConfig(level=level, format="%(asctime)s %(levelname)s %(message)s")


def publish_checks(mqtt: MQTTClient | None, settings: Settings, publications: list[CheckPublication]) -> None:
    for publication in publications:
        payload_bytes = json.dumps(publication.payload, separators=(",", ":")).encode("utf-8")
        if settings.stdout_only:
            print(json.dumps({"topic": publication.topic, "payload": publication.payload}, indent=2))
            continue
        if mqtt is None:
            raise RuntimeError("MQTT client is required unless UP2DATE_STDOUT_ONLY=true")
        mqtt.publish(publication.topic, payload_bytes, retain=settings.retain_messages)


def load_fixture_snapshot(path: str) -> dict[str, Any]:
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


def load_registry_fixtures(path: str | None) -> dict[str, list[str]]:
    if not path:
        return {}
    with open(path, "r", encoding="utf-8") as handle:
        payload = json.load(handle)
    return {str(key): list(value) for key, value in payload.items()}


def run() -> int:
    configure_logging()
    settings = Settings.from_env()
    registry_client = RegistryClient(
        timeout_seconds=settings.registry_timeout_seconds,
        insecure_registries=settings.insecure_registries,
        fixture_tags=load_registry_fixtures(settings.registry_fixture_path),
    )
    engine = ResolverEngine(settings, registry_client)

    LOG.info(
        "starting up2date-resolver topic_filter=%s fixture=%s",
        settings.snapshot_topic_filter,
        settings.snapshot_fixture_path or "-",
    )

    if settings.snapshot_fixture_path:
        snapshot = load_fixture_snapshot(settings.snapshot_fixture_path)
        publications = engine.process_snapshot(snapshot)
        publish_checks(None, settings, publications)
        return 0

    if settings.stdout_only and not settings.mqtt_host:
        raise RuntimeError("UP2DATE_MQTT_HOST is required for live subscribe mode")
    if not settings.mqtt_host:
        raise RuntimeError("UP2DATE_MQTT_HOST must be set unless a snapshot fixture is used")

    client_id = f"up2date-resolver-{uuid.uuid4().hex[:8]}"
    with MQTTClient(
        host=settings.mqtt_host,
        port=settings.mqtt_port,
        client_id=client_id,
        username=settings.mqtt_username,
        password=settings.mqtt_password,
        read_timeout=settings.read_timeout_seconds,
    ) as mqtt:
        mqtt.subscribe(settings.snapshot_topic_filter)
        LOG.info("subscribed to snapshots on %s", settings.snapshot_topic_filter)

        while True:
            try:
                topic, payload = mqtt.read_publish()
                LOG.info("received snapshot on %s", topic)
                snapshot = json.loads(payload.decode("utf-8"))
                publications = engine.process_snapshot(snapshot)
                publish_checks(mqtt, settings, publications)
                LOG.info("published %s checks for node=%s", len(publications), snapshot.get("agent_id", "unknown"))
                if settings.one_shot:
                    return 0
            except KeyboardInterrupt:
                LOG.info("received interrupt, stopping resolver")
                return 0
            except Exception as exc:
                LOG.exception("resolver loop failed: %s", exc)
                if settings.one_shot:
                    return 1
                time.sleep(1)


def main() -> int:
    try:
        return run()
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
