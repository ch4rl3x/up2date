#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
compose_file="${script_dir}/compose.yml"
config_file="${script_dir}/config.yml"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_command podman
require_command go

if [[ "${1:-}" == "--continuous" ]]; then
  shift
elif [[ "$#" -eq 0 ]]; then
  set -- -once
fi

build_dir="$(mktemp -d "${TMPDIR:-/tmp}/up2date-package-brew-mqtt.XXXXXX")"
binary_path="${build_dir}/up2date"
trap 'rm -rf "${build_dir}"' EXIT

podman compose -f "${compose_file}" up -d mqtt

echo "Using config: ${config_file}"
echo "Building binary: ${binary_path}"
echo "Stop the local broker later with: podman compose -f examples/package-brew-mqtt/compose.yml down"

cd "${repo_root}"
go build -o "${binary_path}" ./cmd/up2date
"${binary_path}" -config "${config_file}" "$@"
