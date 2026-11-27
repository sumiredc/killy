#!/usr/bin/env bash
# Builds each overlay under deploy/overlays with kustomize and checks the result without a cluster:
#   - kubeconform validates every resource against the Kubernetes schemas (strictly: an unknown or
#     mistyped field is an error). Only SealedSecret, which has no schema here, is skipped: any other
#     kind without a schema, such as a mistyped one, fails instead of passing unchecked.
#   - every container image is pinned by digest (`@sha256:`), because the registry allows tags to be
#     overwritten and ARCHITECTURE.md says to deploy only by digest.
#
# Needs kustomize and kubeconform on PATH. Nothing talks to a cluster.
set -euo pipefail

cd "$(dirname "$0")/../.."
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

status=0
for overlay in deploy/overlays/*/; do
  name=$(basename "$overlay")
  echo "== $name"
  kustomize build "$overlay" >"$out/$name.yaml"
  kubeconform -strict -skip SealedSecret -summary "$out/$name.yaml" || status=1
  unpinned=$(grep -E '^[[:space:]]*(- )?image:' "$out/$name.yaml" | grep -v '@sha256:' || true)
  if [ -n "$unpinned" ]; then
    echo "images without a digest in $name:" >&2
    echo "$unpinned" >&2
    status=1
  fi
done
exit "$status"
