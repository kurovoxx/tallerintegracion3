#!/usr/bin/env bash
# Valida que todas las migraciones versionadas apliquen limpiamente contra
# una base de datos Postgres. Se usa como gate de deploy: si cualquier
# migración falla, el script sale con error y CI no construye/sube imágenes.
#
# Uso:
#   DATABASE_URL=postgresql://user:pass@host:port/db?sslmode=disable \
#     scripts/validate-migrations.sh
#
set -euo pipefail

DATABASE_URL="${DATABASE_URL:?Falta DATABASE_URL (postgresql://...)}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Servicios que tienen carpeta de migraciones versionadas.
SERVICES=(auth notes social infacadem)

echo "== Validando migraciones versionadas (gate de deploy) =="
echo "Base de datos: ${DATABASE_URL}"

# Asegurar que existan los schemas de todos los servicios antes de aplicar
# sus migraciones (una Postgres limpia no los tiene; init.sql los crea en
# compose, pero el gate de deploy corre contra una DB vacía).
echo "[schemas] Creando schemas identity, academic, notes, social, ai si no existen"
PGPASSWORD="${PGPASSWORD:-}" psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS academic;
CREATE SCHEMA IF NOT EXISTS notes;
CREATE SCHEMA IF NOT EXISTS social;
CREATE SCHEMA IF NOT EXISTS ai;
SQL

for svc in "${SERVICES[@]}"; do
  MIGRATIONS_DIR="${REPO_ROOT}/back/${svc}/db/migrations"
  if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "[skip] ${svc}: no tiene back/${svc}/db/migrations/"
    continue
  fi

  echo "[apply] ${svc}: migraciones en back/${svc}/db/migrations/"
  shopt -s nullglob
  for f in "$MIGRATIONS_DIR"/*.sql; do
    [ -f "$f" ] || continue
    echo "  -> $(basename "$f")"
    PGPASSWORD="${PGPASSWORD:-}" psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f" >/dev/null
  done
  shopt -u nullglob
done

echo ""
echo "[ok] Todas las migraciones aplicaron limpiamente."
