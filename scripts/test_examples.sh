#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform not found in PATH" >&2
  exit 1
fi

TFMODMAKE_BIN="$(mktemp -t tfmodmake.XXXXXX)"
cleanup() {
  rm -f "$TFMODMAKE_BIN"
}
trap cleanup EXIT

go build -o "$TFMODMAKE_BIN" "$ROOT_DIR/cmd/tfmodmake"

run_case() {
  local name="$1"
  local spec="$2"
  local resource="$3"

  echo "== $name =="

  local workdir
  workdir="$(mktemp -d -t tfmodmake_example.XXXXXX)"

  (cd "$workdir" && "$TFMODMAKE_BIN" -spec "$spec" -resource "$resource" >/dev/null)
  (cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
  (cd "$workdir" && terraform validate -no-color >/dev/null)

  echo "ok"
}

run_keyvault_case() {
  echo "== vaults =="

  local workdir
  workdir="$(mktemp -d -t tfmodmake_example.XXXXXX)"

  mkdir -p "$workdir/modules/secrets"

  # Base module: Microsoft.KeyVault/vaults
  (
    cd "$workdir" &&
      "$TFMODMAKE_BIN" \
        -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2025-05-01/openapi.json" \
        -resource "Microsoft.KeyVault/vaults" \
        >/dev/null
  )

  # Secrets submodule: Microsoft.KeyVault/vaults/secrets
  (
    cd "$workdir/modules/secrets" &&
      "$TFMODMAKE_BIN" \
        -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2024-11-01/secrets.json" \
        -resource "Microsoft.KeyVault/vaults/secrets" \
        -local-name "secret_body" \
        >/dev/null
  )

  # Parent module wrapper generation for secrets submodule
  (cd "$workdir" && "$TFMODMAKE_BIN" addsub modules/secrets >/dev/null)

  (cd "$workdir" && terraform init -backend=false -input=false -no-color >/dev/null)
  (cd "$workdir" && terraform validate -no-color >/dev/null)

  echo "ok"
}

run_case \
  "managedClusters" \
  "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/containerservice/resource-manager/Microsoft.ContainerService/aks/stable/2025-10-01/managedClusters.json" \
  "Microsoft.ContainerService/managedClusters"

run_case \
  "managedEnvironments" \
  "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json" \
  "Microsoft.App/managedEnvironments"

run_keyvault_case
