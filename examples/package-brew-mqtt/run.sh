#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
compose_file="${script_dir}/compose.yml"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

pick_package_names() {
  if [[ -n "${UP2DATE_COLLECTOR_PACKAGE_NAMES:-}" ]]; then
    printf '%s\n' "${UP2DATE_COLLECTOR_PACKAGE_NAMES}"
    return
  fi

  local preferred
  for preferred in gettext ripgrep go python@3.12 ca-certificates; do
    if brew list --versions --formula "$preferred" >/dev/null 2>&1; then
      printf '%s\n' "$preferred"
      return
    fi
  done

  local first_installed
  first_installed="$(brew list --formula | head -n 1 | tr -d '\r')"
  if [[ -n "${first_installed}" ]]; then
    printf '%s\n' "${first_installed}"
    return
  fi

  echo "No installed Homebrew formula found. Install one first or set UP2DATE_COLLECTOR_PACKAGE_NAMES." >&2
  exit 1
}

require_command podman
require_command brew
require_command go

if [[ "${1:-}" == "--continuous" ]]; then
  shift
elif [[ "$#" -eq 0 ]]; then
  set -- -once
fi

package_names="$(pick_package_names)"

podman compose -f "${compose_file}" up -d mqtt

export UP2DATE_NODE_ID="${UP2DATE_NODE_ID:-$(hostname -s)}"
export UP2DATE_INTERVAL="${UP2DATE_INTERVAL:-1m}"
export UP2DATE_COLLECTOR_TYPE="package"
export UP2DATE_COLLECTOR_PACKAGE_MANAGER="${UP2DATE_COLLECTOR_PACKAGE_MANAGER:-brew}"
export UP2DATE_COLLECTOR_PACKAGE_NAMES="${package_names}"
export UP2DATE_RESOLVER_TYPE="${UP2DATE_RESOLVER_TYPE:-brew_formula}"
export UP2DATE_PUBLISHER_TYPE="mqtt"
export UP2DATE_PUBLISHER_MQTT_HOST="${UP2DATE_PUBLISHER_MQTT_HOST:-127.0.0.1}"
export UP2DATE_PUBLISHER_MQTT_PORT="${UP2DATE_PUBLISHER_MQTT_PORT:-1883}"

echo "Using Homebrew package(s): ${UP2DATE_COLLECTOR_PACKAGE_NAMES}"
echo "Publishing to MQTT broker at ${UP2DATE_PUBLISHER_MQTT_HOST}:${UP2DATE_PUBLISHER_MQTT_PORT}"
echo "Stop the local broker later with: podman compose -f examples/package-brew-mqtt/compose.yml down"

cd "${repo_root}"
go run ./cmd/up2date "$@"
