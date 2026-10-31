#!/bin/bash
set -e
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

if [ "${1:-}" = "--e2e" ]; then
  exec "$PROJECT_ROOT/scripts/run-e2e-local.sh"
fi

# Detecta si necesita sudo para docker
DOCKER="docker"
if ! docker ps >/dev/null 2>&1; then
  DOCKER="sudo docker"
fi
COMPOSE="$DOCKER compose"

echo "=== TI3 run-local (puertos locales) ==="
echo "Usa front/lib/core/services/api_config.dart defaults http://localhost:8085"
echo "y front/nginx.conf /api-* -> auth:8080 sin dominio"
echo ""

# Asegura .env principal con DATABASE_URL etc. existe
if [ ! -f .env ]; then
  echo "ERROR: .env no encontrado en $PROJECT_ROOT/.env (necesario para DOCKER_DATABASE_URL, JWT...)"
  exit 1
fi

# Fuerza build local sin dominio (ignora API_BASE_URL de .env si existe)
# Descomenta si quieres forzar relativo para web sin CORS:
# export API_BASE_URL=/api-auth
# export NOTES_BASE_URL=/api-notes
# export SOCIAL_BASE_URL=/api-social
# export ACADEMIC_BASE_URL=/api-academic

echo ">> Build front local (http://localhost:8085)..."
$COMPOSE build --no-cache front

echo ">> Rebuild backends (incluye las correcciones de Notes)..."
$COMPOSE build notes auth social infacadem

echo ">> Levanta postgres + auth + notes + social + infacadem + front..."
$COMPOSE up -d

echo ""
$COMPOSE ps
echo ""
echo "Logs: $COMPOSE logs -f"
echo "Front: http://localhost:8086  (nginx 80->front, proxea /api-auth -> auth:8080)"
echo "Auth:  http://localhost:8085/health  (directo host 8085->8080)"
echo "Notes: http://localhost:8082/health"
echo "Social:http://localhost:8083/health"
echo "Academic directo: http://localhost:8087/health  (host 8087->8081, 8081 reservado)"
echo "Academic vía nginx: http://localhost:8086/api-academic/health  (-> academic:8081)"
echo ""
echo "Test:"
echo "  curl -v http://localhost:8085/health"
echo "  curl -v http://localhost:8086/api-auth/health"
echo "  curl -v http://localhost:8087/health"
echo "  curl -v http://localhost:8086/api-academic/health"
