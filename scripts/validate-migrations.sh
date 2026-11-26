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
INIT_SQL_PATH="${INIT_SQL_PATH:-${REPO_ROOT}/docker/postgres/init.sql}"

# Servicios que tienen carpeta de migraciones versionadas.
SERVICES=(auth notes social infacadem)

echo "== Validando migraciones versionadas (gate de deploy) =="
echo "Base de datos: ${DATABASE_URL}"

# 1. Esquema base: aplicar docker/postgres/init.sql primero. Las migraciones
#    versionadas de auth/notes/social asumen que tablas base como
#    identity.users ya existen (creadas por init.sql), y añaden sobre ellas.
echo "[base] Aplicando docker/postgres/init.sql (esquema base de todos los servicios)"
PGPASSWORD="${PGPASSWORD:-}" psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$INIT_SQL_PATH" >/dev/null

# 2. Migraciones versionadas por servicio (se aplican encima del esquema base).
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
