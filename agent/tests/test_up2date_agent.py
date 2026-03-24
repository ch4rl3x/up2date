from __future__ import annotations

import datetime as dt
import json
import unittest
from unittest import mock
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "app"))

import up2date_agent


FIXTURE_PATH = Path(__file__).resolve().parents[1] / "fixtures" / "docker_containers.json"


class Up2DateAgentTests(unittest.TestCase):
    def test_parse_image_reference_with_registry_port(self) -> None:
        image_name, image_tag = up2date_agent.parse_image_reference("registry.internal:5000/legacy/app:2026.03.1")
        self.assertEqual(image_name, "registry.internal:5000/legacy/app")
        self.assertEqual(image_tag, "2026.03.1")

    def test_build_snapshot_uses_label_version_and_excludes_self(self) -> None:
        with FIXTURE_PATH.open("r", encoding="utf-8") as handle:
            containers = json.load(handle)

        snapshot = up2date_agent.build_snapshot(
            "docker-host-01",
            containers,
            observed_at=dt.datetime(2026, 3, 24, 18, 42, tzinfo=dt.timezone.utc),
            node_name="Docker Host 01",
            self_container_prefix="abc123",
        )

        self.assertEqual(snapshot["agent_id"], "docker-host-01")
        self.assertEqual(snapshot["node_name"], "Docker Host 01")
        self.assertEqual(snapshot["observed_at"], "2026-03-24T18:42:00Z")
        self.assertEqual(len(snapshot["services"]), 3)

        paperless = next(service for service in snapshot["services"] if service["service_name"] == "paperless")
        self.assertEqual(paperless["detected_version"], "2.14.7")
        self.assertEqual(paperless["detected_version_source"], "container_label")
        self.assertEqual(paperless["version_label_key"], "org.opencontainers.image.version")

        legacy = next(service for service in snapshot["services"] if service["service_name"] == "legacy-app")
        self.assertFalse(legacy["running"])
        self.assertEqual(legacy["image_name"], "registry.internal:5000/legacy/app")
        self.assertEqual(legacy["image_tag"], "2026.03.1")
        self.assertEqual(legacy["detected_version"], "2026.03.1")
        self.assertEqual(legacy["detected_version_source"], "image_tag")

    def test_build_snapshot_excludes_ignore_labeled_containers(self) -> None:
        with FIXTURE_PATH.open("r", encoding="utf-8") as handle:
            containers = json.load(handle)

        snapshot = up2date_agent.build_snapshot(
            "docker-host-01",
            containers,
            observed_at=dt.datetime(2026, 3, 24, 18, 42, tzinfo=dt.timezone.utc),
            exclude_label_selectors=("up2date.ignore=true",),
        )

        service_names = {service["service_name"] for service in snapshot["services"]}
        self.assertNotIn("up2date-agent", service_names)
        self.assertEqual(len(snapshot["services"]), 3)

    def test_publish_cycle_sends_status_and_snapshot(self) -> None:
        settings = up2date_agent.Settings(node_id="docker-host-01", mqtt_host="broker", retain_messages=True)

        snapshot = {
            "schema_version": 1,
            "kind": "docker_node_snapshot",
            "agent_id": "docker-host-01",
            "node_name": "Docker Host 01",
            "observed_at": "2026-03-24T18:42:00Z",
            "services": [],
        }
        status = up2date_agent.build_status(snapshot)

        published: list[tuple[str, dict, bool]] = []

        class FakeMQTTClient:
            def __init__(self, *args, **kwargs) -> None:
                self.args = args
                self.kwargs = kwargs

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb) -> None:
                return None

            def publish(self, topic: str, payload: bytes, retain: bool = False) -> None:
                published.append((topic, json.loads(payload.decode("utf-8")), retain))

        with mock.patch.object(up2date_agent, "MQTTClient", FakeMQTTClient):
            up2date_agent.publish_cycle(settings, snapshot, status)

        self.assertEqual(len(published), 2)
        self.assertEqual(published[0][0], "up2date/nodes/docker-host-01/status")
        self.assertEqual(published[1][0], "up2date/nodes/docker-host-01/snapshot")
        self.assertTrue(published[0][2])
        self.assertTrue(published[1][2])


if __name__ == "__main__":
    unittest.main()
