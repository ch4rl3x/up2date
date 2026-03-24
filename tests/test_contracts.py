from __future__ import annotations

import datetime as dt
import json
import sys
import unittest
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
AGENT_APP = ROOT / "agent" / "app"
RESOLVER_APP = ROOT / "resolver" / "app"

sys.path.insert(0, str(AGENT_APP))
sys.path.insert(0, str(RESOLVER_APP))

import up2date_agent
import up2date_resolver


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def parse_rfc3339(value: str) -> dt.datetime:
    return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))


def assert_matches_schema(testcase: unittest.TestCase, payload: Any, schema: dict[str, Any], path: str = "$") -> None:
    schema_type = schema.get("type")
    if isinstance(schema_type, list):
        if any(_matches_type(payload, candidate) for candidate in schema_type):
            active_type = next(candidate for candidate in schema_type if _matches_type(payload, candidate))
            _assert_type_specific(testcase, payload, schema, active_type, path)
            return
        testcase.fail(f"{path} does not match any allowed types {schema_type!r}: {payload!r}")

    if schema_type is not None and not _matches_type(payload, schema_type):
        testcase.fail(f"{path} expected type {schema_type!r}, got {type(payload).__name__}")

    _assert_type_specific(testcase, payload, schema, schema_type, path)


def _matches_type(payload: Any, expected: str) -> bool:
    if expected == "object":
        return isinstance(payload, dict)
    if expected == "array":
        return isinstance(payload, list)
    if expected == "string":
        return isinstance(payload, str)
    if expected == "integer":
        return isinstance(payload, int) and not isinstance(payload, bool)
    if expected == "boolean":
        return isinstance(payload, bool)
    if expected == "null":
        return payload is None
    return True


def _assert_type_specific(
    testcase: unittest.TestCase,
    payload: Any,
    schema: dict[str, Any],
    schema_type: str | None,
    path: str,
) -> None:
    if "const" in schema:
        testcase.assertEqual(payload, schema["const"], msg=f"{path} must equal const value")

    if "enum" in schema:
        testcase.assertIn(payload, schema["enum"], msg=f"{path} must be one of {schema['enum']!r}")

    if schema_type == "string" and schema.get("format") == "date-time":
        try:
            parse_rfc3339(payload)
        except ValueError as exc:
            testcase.fail(f"{path} is not a valid RFC3339 date-time: {exc}")

    if schema_type == "integer":
        minimum = schema.get("minimum")
        if minimum is not None:
            testcase.assertGreaterEqual(payload, minimum, msg=f"{path} must be >= {minimum}")

    if schema_type == "array":
        item_schema = schema.get("items")
        if item_schema is not None:
            for index, item in enumerate(payload):
                assert_matches_schema(testcase, item, item_schema, f"{path}[{index}]")

    if schema_type == "object":
        required = schema.get("required", [])
        for key in required:
            testcase.assertIn(key, payload, msg=f"{path} missing required key {key!r}")

        properties = schema.get("properties", {})
        for key, value in payload.items():
            if key not in properties:
                if schema.get("additionalProperties", True) is False:
                    testcase.fail(f"{path} contains unexpected key {key!r}")
                continue
            assert_matches_schema(testcase, value, properties[key], f"{path}.{key}")


class ContractSchemaTests(unittest.TestCase):
    def setUp(self) -> None:
        self.snapshot_schema = load_json(ROOT / "schemas" / "mqtt-node-snapshot.schema.json")
        self.status_schema = load_json(ROOT / "schemas" / "agent-status.schema.json")
        self.check_schema = load_json(ROOT / "schemas" / "service-check.schema.json")

    def test_agent_snapshot_matches_snapshot_schema(self) -> None:
        containers = load_json(ROOT / "agent" / "fixtures" / "docker_containers.json")
        snapshot = up2date_agent.build_snapshot(
            "docker-host-01",
            containers,
            observed_at=dt.datetime(2026, 3, 24, 18, 42, tzinfo=dt.timezone.utc),
            node_name="Docker Host 01",
            self_container_prefix="abc123",
        )

        assert_matches_schema(self, snapshot, self.snapshot_schema)

    def test_agent_status_matches_status_schema(self) -> None:
        containers = load_json(ROOT / "agent" / "fixtures" / "docker_containers.json")
        snapshot = up2date_agent.build_snapshot(
            "docker-host-01",
            containers,
            observed_at=dt.datetime(2026, 3, 24, 18, 42, tzinfo=dt.timezone.utc),
            node_name="Docker Host 01",
            self_container_prefix="abc123",
        )
        status = up2date_agent.build_status(snapshot)

        assert_matches_schema(self, status, self.status_schema)

    def test_fixture_snapshot_matches_snapshot_schema(self) -> None:
        snapshot = load_json(ROOT / "resolver" / "fixtures" / "node_snapshot.json")
        assert_matches_schema(self, snapshot, self.snapshot_schema)

    def test_resolver_publications_match_service_check_schema(self) -> None:
        settings = up2date_resolver.Settings(mqtt_host="broker")
        registry_fixtures = load_json(ROOT / "resolver" / "fixtures" / "registry_tags.json")
        engine = up2date_resolver.ResolverEngine(
            settings,
            up2date_resolver.RegistryClient(fixture_tags=registry_fixtures),
        )

        snapshot = load_json(ROOT / "resolver" / "fixtures" / "node_snapshot.json")
        publications = engine.process_snapshot(snapshot)

        self.assertGreaterEqual(len(publications), 1)
        for publication in publications:
            assert_matches_schema(self, publication.payload, self.check_schema)


if __name__ == "__main__":
    unittest.main()
