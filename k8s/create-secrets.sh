#!/usr/bin/env bash
# Template, no valores reales acá (no se versiona secretos).
# Completar y correr una vez antes del primer `kubectl apply -f k8s/manifests.yaml`.
set -euo pipefail

kubectl create secret generic ti3-secrets -n student-brojas \
  --from-literal=DATABASE_URL="postgresql://postgres:<POSTGRES_PASSWORD de postgres-auth>@postgres:5432/postgres?sslmode=disable" \
  --from-literal=DIRECT_URL="postgresql://postgres:<POSTGRES_PASSWORD de postgres-auth>@postgres:5432/postgres?sslmode=disable" \
  --from-literal=JWT_SECRET="<secreto HS256 comun a auth/notes/social: openssl rand -hex 32>" \
  --from-literal=INTERNAL_API_KEY="dev-internal-key-change-me" \
  --from-literal=GOOGLE_CLIENT_ID="<pegar de .env / Cloud Console>" \
  --from-literal=GOOGLE_CLIENT_SECRET="<pegar de .env / Cloud Console>" \
  --from-literal=GOOGLE_REDIRECT_URI="https://ti3-brojas.dev.censei.cl/auth/google/callback" \
  --dry-run=client -o yaml | kubectl apply -f -

# Con tu propio PAT GitHub (scope read:packages), para que el cluster baje las imágenes privadas:
kubectl create secret docker-registry ghcr-cred -n student-brojas \
  --docker-server=ghcr.io \
  --docker-username=pvcdf \
  --docker-password="<tu PAT>" \
  --dry-run=client -o yaml | kubectl apply -f -
