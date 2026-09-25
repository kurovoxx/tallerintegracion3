#!/usr/bin/env bash
# Template, no valores reales acá (no se versiona secretos).
# Completar y correr una vez antes del primer `kubectl apply -f k8s/manifests.yaml`.
set -euo pipefail

kubectl create secret generic ti3-secrets -n student-brojas \
  --from-literal=DATABASE_URL="<pegar de .env>" \
  --from-literal=DIRECT_URL="<pegar de .env>" \
  --from-literal=JWT="<pegar de .env>" \
  --from-literal=JWT_SECRET="<pegar del docker-compose.yml, bloque social>" \
  --from-literal=SUPABASE_URL="<pegar de .env>" \
  --from-literal=DISCOVERY_URL="<pegar de .env>" \
  --from-literal=INTERNAL_API_KEY="dev-internal-key-change-me" \
  --from-literal=GOOGLE_CLIENT_ID="<pendiente>" \
  --from-literal=GOOGLE_CLIENT_SECRET="<pendiente>" \
  --from-literal=GOOGLE_REDIRECT_URI="<pendiente>" \
  --dry-run=client -o yaml | kubectl apply -f -

# Con tu propio PAT GitHub (scope read:packages), para que el cluster baje las imágenes privadas:
kubectl create secret docker-registry ghcr-cred -n student-brojas \
  --docker-server=ghcr.io \
  --docker-username=pvcdf \
  --docker-password="<tu PAT>" \
  --dry-run=client -o yaml | kubectl apply -f -
