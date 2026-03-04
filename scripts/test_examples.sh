#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform not found in PATH" >&2
  exit 1
fi

TFMODMAKE_BIN="$(mktemp -t tfmodmake.XXXXXX)"

WORKDIRS=()
cleanup() {
  rm -f "$TFMODMAKE_BIN"

  for dir in "${WORKDIRS[@]:-}"; do
    if [[ -n "${dir}" && -d "${dir}" ]]; then
      rm -rf "${dir}"
    fi
  done
}
trap cleanup EXIT

go build -o "$TFMODMAKE_BIN" "$ROOT_DIR/cmd/tfmodmake"

run_case() {
  local name="$1"
  local resource="$2"
  shift 2
  local extra_flags=("$@")

  echo "== $name =="

  local workdir
  workdir="$(mktemp -d -t tfmodmake_example.XXXXXX)"
  WORKDIRS+=("$workdir")

  (cd "$workdir" && "$TFMODMAKE_BIN" gen --resource "$resource" "${extra_flags[@]}" >/dev/null)
  (cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
  (cd "$workdir" && terraform validate -no-color >/dev/null)

  echo "ok"
}

run_keyvault_case() {
  echo "== vaults =="

  local workdir
  workdir="$(mktemp -d -t tfmodmake_example.XXXXXX)"
  WORKDIRS+=("$workdir")

  mkdir -p "$workdir/modules/secrets"

  # Base module: Microsoft.KeyVault/vaults
  (
    cd "$workdir" &&
      "$TFMODMAKE_BIN" \
        gen \
        --resource "Microsoft.KeyVault/vaults" \
        >/dev/null
  )

  # Secrets submodule: Microsoft.KeyVault/vaults/secrets
  (
    cd "$workdir/modules/secrets" &&
      "$TFMODMAKE_BIN" \
        gen \
        --resource "Microsoft.KeyVault/vaults/secrets" \
        --local-name "secret_body" \
        >/dev/null
  )

  # Parent module wrapper generation for secrets submodule
  (cd "$workdir" && "$TFMODMAKE_BIN" add submodule modules/secrets >/dev/null)

  (cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
  (cd "$workdir" && terraform validate -no-color >/dev/null)

  echo "ok"
}

run_update_case() {
  echo "== managedClusters (update) =="

  local workdir
  workdir="$(mktemp -d -t tfmodmake_example.XXXXXX)"
  WORKDIRS+=("$workdir")

  local resource="Microsoft.ContainerService/managedClusters"

  # Step 1: Generate with the previous stable API version
  (cd "$workdir" && "$TFMODMAKE_BIN" gen --resource "$resource" --api-version "2025-09-01" >/dev/null)
  (cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
  (cd "$workdir" && terraform validate -no-color >/dev/null)

  # Step 2: Update to the latest stable version
  (cd "$workdir" && "$TFMODMAKE_BIN" update --api-version "2025-10-01" >/dev/null)
  (cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
  (cd "$workdir" && terraform validate -no-color >/dev/null)

  echo "ok"
}

# Basic generation test
run_case \
  "managedClusters" \
  "Microsoft.ContainerService/managedClusters"

# AVM generation test
echo "== managedEnvironments (gen avm) =="
workdir="$(mktemp -d -t tfmodmake_example.XXXXXX)"
WORKDIRS+=("$workdir")
(cd "$workdir" && "$TFMODMAKE_BIN" gen avm \
  --resource Microsoft.App/managedEnvironments \
  --include-preview >/dev/null)
(cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
(cd "$workdir" && terraform validate -no-color >/dev/null)
echo "ok"

# KeyVault with submodule test
run_keyvault_case

# Update flow test
run_update_case
