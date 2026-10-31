#!/bin/bash
# Entorno E2E 100% local y aislado (Docker Desktop + Git Bash/MSYS).
#
# - PostgreSQL local en contenedor dedicado (volumen ti3-e2e-pgdata).
# - Notes con STORAGE_MODE=mock (no Google Drive real).
# - Social con STREAM_MODE=mock (no GetStream real).
# - Sin .env raíz: docker-compose.e2e.yml usa credenciales de prueba literales.
# - Contenedores/red/puertos propios (18082/18083/18085/18087/15432).
#
# Uso:
#   bash scripts/run-e2e-local.sh            # levanta/actualiza
#   bash scripts/run-e2e-local.sh logs       # logs -f
#   bash scripts/run-e2e-local.sh down       # detiene (conserva datos E2E)
#   bash scripts/run-e2e-local.sh down-v     # detiene y borra SOLO datos E2E
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.e2e.yml"
PROJECT="ti3-e2e"

cd "$PROJECT_ROOT"

if ! docker info >/dev/null 2>&1; then
  echo "ERROR: Docker no está corriendo. Inicia Docker Desktop y reintenta."
  exit 1
fi

case "${1:-up}" in
  down)
    docker compose -f "$COMPOSE_FILE" -p "$PROJECT" down
    exit 0
    ;;
  down-v)
    docker compose -f "$COMPOSE_FILE" -p "$PROJECT" down -v
    exit 0
    ;;
  logs)
    docker compose -f "$COMPOSE_FILE" -p "$PROJECT" logs -f
    exit 0
    ;;
  up|"")
    ;;
  *)
    echo "Uso: $0 [up|logs|down|down-v]"
    exit 1
    ;;
esac

echo ">> Levantando entorno E2E aislado (build incluido)..."
docker compose -f "$COMPOSE_FILE" -p "$PROJECT" up -d --build

wait_health() {
  local name="$1" url="$2"
  for _ in $(seq 1 60); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "   $name OK -> $url"
      return 0
    fi
    sleep 2
  done
  echo "ERROR: $name no respondió en $url"
  return 1
}

echo ">> Esperando salud de servicios..."
wait_health "Auth" "http://localhost:18085/health"
wait_health "Notes" "http://localhost:18082/health"
wait_health "Social" "http://localhost:18083/health"
wait_health "Academic" "http://localhost:18087/health"

echo ">> Verificando que Notes NO use Drive real..."
DRIVE_MODE="$(curl -fsS http://localhost:18082/health/drive || true)"
echo "   $DRIVE_MODE"
case "$DRIVE_MODE" in
  *'"mock"'*) ;;
  *) echo "ERROR: Notes no está en STORAGE_MODE=mock"; exit 1 ;;
esac

DB_BASH="$PROJECT_ROOT/.local-e2e/sigma_academy_e2e.db"
mkdir -p "$(dirname "$DB_BASH")"
if command -v cygpath >/dev/null 2>&1; then
  DB_DEFINE="$(cygpath -w "$DB_BASH")"
else
  DB_DEFINE="$DB_BASH"
fi

echo ""
echo "=== Entorno E2E listo (aislado) ==="
echo "Auth:     http://localhost:18085"
echo "Notes:    http://localhost:18082  (Drive mock)"
echo "Social:   http://localhost:18083  (Stream mock)"
echo "Academic: http://localhost:18087"
echo "Postgres: localhost:15432 (contenedor ti3-e2e-postgres)"
echo "SQLite:   $DB_DEFINE"
echo ""
echo "Front (otra terminal, en $PROJECT_ROOT/front):"
echo "flutter run -d windows \\"
echo "  --dart-define=API_BASE_URL=http://localhost:18085 \\"
echo "  --dart-define=NOTES_BASE_URL=http://localhost:18082 \\"
echo "  --dart-define=SOCIAL_BASE_URL=http://localhost:18083 \\"
echo "  --dart-define=ACADEMIC_BASE_URL=http://localhost:18087 \\"
echo "  --dart-define=APP_DB_PATH=\"$DB_DEFINE\""
echo ""
echo "Credenciales: registra un usuario nuevo en la app (email/password local)."
echo "Teardown:     bash scripts/run-e2e-local.sh down"
