from __future__ import annotations

import json
import unittest
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "app"))

import up2date_resolver


FIXTURE_PATH = Path(__file__).resolve().parents[1] / "fixtures" / "node_snapshot.json"


class FakeRegistryClient:
    def __init__(self) -> None:
        self.calls: list[str] = []

    def list_tags(self, image_name: str):
        self.calls.append(image_name)
        fixtures = {
            "docker.io/library/nginx": (
                ["1.26-alpine", "1.27-alpine", "1.28-alpine", "latest"],
                up2date_resolver.parse_registry_reference(image_name),
            ),
            "docker.io/library/eclipse-mosquitto": (
                ["1.6.15", "2.0.20", "2.1.2", "2.1.4", "latest"],
                up2date_resolver.parse_registry_reference(image_name),
            ),
        }
        return fixtures[image_name]


class ChallengeRegistryClient(up2date_resolver.RegistryClient):
    def __init__(self) -> None:
        super().__init__()
        self.calls: list[tuple[str, str, str, str | None, str | None]] = []

    def _request(self, host: str, method: str, path: str, *, token: str | None, scheme: str | None = None):
        self.calls.append((host, method, path, token, scheme))

        if host == "registry-1.docker.io" and token is None:
            return up2date_resolver.HTTPResult(
                status=401,
                headers={
                    "www-authenticate": (
                        'Bearer realm="https://auth.docker.io/token",'
                        'service="registry.docker.io",'
                        'scope="repository:jellyfin/jellyfin:pull"'
                    )
                },
                body=b"",
            )

        if host == "auth.docker.io":
            return up2date_resolver.HTTPResult(
                status=200,
                headers={},
                body=json.dumps({"token": "test-token"}).encode("utf-8"),
            )

        if host == "registry-1.docker.io" and token == "test-token":
            return up2date_resolver.HTTPResult(
                status=200,
                headers={},
                body=json.dumps({"tags": ["10.11.6", "10.11.7", "latest"]}).encode("utf-8"),
            )

        raise AssertionError(f"Unexpected request: {(host, method, path, token, scheme)}")


class Up2DateResolverTests(unittest.TestCase):
    def test_parse_registry_reference_defaults_to_docker_hub_library(self) -> None:
        reference = up2date_resolver.parse_registry_reference("nginx")
        self.assertEqual(reference.registry_host, "registry-1.docker.io")
        self.assertEqual(reference.repository, "library/nginx")

    def test_determine_resolution_for_suffix_track(self) -> None:
        service = {
            "service_name": "app",
            "image_name": "docker.io/library/nginx",
            "image_tag": "1.27-alpine",
            "detected_version": "1.27-alpine",
            "detected_version_source": "image_tag",
        }
        tags = ["1.26-alpine", "1.27-alpine", "1.28-alpine", "1.28-bookworm", "latest"]
        reference = up2date_resolver.parse_registry_reference(service["image_name"])

        resolution = up2date_resolver.determine_resolution(service, tags, reference)

        self.assertEqual(resolution.latest_version, "1.28-alpine")
        self.assertEqual(resolution.status, "outdated")
        self.assertTrue(resolution.update_available)
        self.assertEqual(resolution.comparison_track, "same_suffix_same_shape")

    def test_determine_resolution_for_floating_major_tag(self) -> None:
        service = {
            "service_name": "mqtt",
            "image_name": "docker.io/library/eclipse-mosquitto",
            "image_tag": "2",
            "detected_version": "2.1.2",
            "detected_version_source": "container_label",
        }
        tags = ["1.6.15", "2.0.20", "2.1.2", "2.1.4", "latest"]
        reference = up2date_resolver.parse_registry_reference(service["image_name"])

        resolution = up2date_resolver.determine_resolution(service, tags, reference)

        self.assertEqual(resolution.latest_version, "2.1.4")
        self.assertEqual(resolution.status, "outdated")
        self.assertTrue(resolution.update_available)
        self.assertEqual(resolution.comparison_track, "same_major_same_shape")

    def test_determine_resolution_ignores_incompatible_numeric_tracks(self) -> None:
        service = {
            "service_name": "jellyfin",
            "image_name": "jellyfin/jellyfin",
            "image_tag": "10.11.6",
            "detected_version": "10.11.6",
            "detected_version_source": "container_label",
        }
        tags = ["10.11.6", "10.11.7", "2026032305", "latest"]
        reference = up2date_resolver.parse_registry_reference(service["image_name"])

        resolution = up2date_resolver.determine_resolution(service, tags, reference)

        self.assertEqual(resolution.latest_version, "10.11.7")
        self.assertEqual(resolution.status, "outdated")
        self.assertTrue(resolution.update_available)
        self.assertEqual(resolution.comparison_track, "same_suffix_same_shape")

    def test_engine_builds_publications_for_each_service(self) -> None:
        snapshot = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
        settings = up2date_resolver.Settings(mqtt_host="broker")
        engine = up2date_resolver.ResolverEngine(settings, FakeRegistryClient())

        publications = engine.process_snapshot(snapshot)

        self.assertEqual(len(publications), 2)
        self.assertEqual(publications[0].topic, "up2date/nodes/local-dev-node/checks/app")
        self.assertEqual(publications[1].topic, "up2date/nodes/local-dev-node/checks/mqtt")

        app_check = publications[0].payload
        self.assertEqual(app_check["status"], "outdated")
        self.assertEqual(app_check["latest_version"], "1.28-alpine")
        self.assertNotIn("snapshot_observed_at", app_check)
        self.assertNotIn("current_version_source", app_check)
        self.assertNotIn("resolver", app_check)
        self.assertNotIn("latest_version_source", app_check)
        self.assertNotIn("comparison_track", app_check)
        self.assertNotIn("registry", app_check)
        self.assertNotIn("repository", app_check)
        self.assertNotIn("image_tag", app_check)

        mqtt_check = publications[1].payload
        self.assertEqual(mqtt_check["status"], "outdated")
        self.assertEqual(mqtt_check["latest_version"], "2.1.4")
        self.assertEqual(engine.registry_client.calls, ["docker.io/library/nginx", "docker.io/library/eclipse-mosquitto"])

    def test_engine_skips_unsupported_services_and_reuses_registry_results_per_snapshot(self) -> None:
        snapshot = {
            "schema_version": 1,
            "kind": "docker_node_snapshot",
            "agent_id": "solar",
            "node_name": "Solar",
            "observed_at": "2026-03-24T20:34:56Z",
            "services": [
                {
                    "service_name": "grafana",
                    "image_name": "grafana/grafana",
                    "image_tag": "11.2.2",
                    "detected_version": "11.2.2",
                },
                {
                    "service_name": "grafana-internal",
                    "image_name": "grafana/grafana",
                    "image_tag": "11.2.2",
                    "detected_version": "11.2.2",
                },
                {
                    "service_name": "solar",
                    "image_name": "solar",
                    "detected_version": "unknown",
                },
            ],
        }

        class SnapshotRegistryClient:
            def __init__(self) -> None:
                self.calls: list[str] = []

            def list_tags(self, image_name: str):
                self.calls.append(image_name)
                return ["11.2.2", "11.6.0", "latest"], up2date_resolver.parse_registry_reference(image_name)

        registry = SnapshotRegistryClient()
        settings = up2date_resolver.Settings(mqtt_host="broker")
        engine = up2date_resolver.ResolverEngine(settings, registry)

        publications = engine.process_snapshot(snapshot)

        self.assertEqual(len(publications), 3)
        self.assertEqual(registry.calls, ["grafana/grafana"])
        self.assertEqual(publications[0].payload["latest_version"], "11.6.0")
        self.assertEqual(publications[1].payload["latest_version"], "11.6.0")
        self.assertEqual(publications[2].payload["status"], "unsupported")
        self.assertEqual(
            publications[2].payload["reason"],
            "Current version is not parseable as a numeric release tag",
        )

    def test_registry_client_accepts_lowercase_auth_header_keys(self) -> None:
        client = ChallengeRegistryClient()

        tags, reference = client.list_tags("jellyfin/jellyfin")

        self.assertEqual(reference.registry_host, "registry-1.docker.io")
        self.assertEqual(reference.repository, "jellyfin/jellyfin")
        self.assertEqual(tags, ["10.11.6", "10.11.7", "latest"])
        self.assertEqual(client.calls[0][0], "registry-1.docker.io")
        self.assertEqual(client.calls[1][0], "auth.docker.io")
        self.assertEqual(client.calls[2][3], "test-token")


if __name__ == "__main__":
    unittest.main()
