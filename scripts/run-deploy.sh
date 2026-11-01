#!/bin/bash
set -e
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

DOCKER="docker"
if ! docker ps >/dev/null 2>&1; then
  DOCKER="sudo docker"
fi
COMPOSE="$DOCKER compose"

echo "=== TI3 run-deploy (dominio del Ingress) ==="
echo "Usa dominio público para que el front pegue a k8s pillan sin localhost"
echo ""

# Dominio por defecto tomado de .env.prod o front/.env.prod.example
ENV_FILE=".env"
if [ -f front/.env.prod.example ]; then
  ENV_EXAMPLE="front/.env.prod.example"
else
  ENV_EXAMPLE=".env"
fi

# Si existe .env.prod en root, úsalo; si no, usa el example de front
if [ -f .env.prod ]; then
  DEPLOY_ENV=".env.prod"
elif [ -f front/.env.prod.example ]; then
  DEPLOY_ENV="front/.env.prod.example"
  echo "Usando $DEPLOY_ENV (copia a .env.prod para editar dominio si cambia)"
else
  echo "ERROR: No se encontró .env.prod ni $ENV_EXAMPLE con API_BASE_URL"
  exit 1
fi

echo ">> Env deploy: $DEPLOY_ENV"
cat "$DEPLOY_ENV" | grep -E "API_BASE_URL|NOTES_BASE_URL|SOCIAL_BASE_URL" || true
echo ""

# Carga vars de deploy para el build (además del .env principal con DOCKER_DATABASE_URL)
# Si .env principal ya tiene API_BASE_URL, .env.prod lo sobreescribe
set -a
# shellcheck disable=SC1090
source "$DEPLOY_ENV"
set +a

echo ">> Build front con dominio $API_BASE_URL ..."
$COMPOSE build --no-cache front

echo ">> Levanta solo front (backends en k8s pillan via $API_BASE_URL, no local)..."
$COMPOSE up -d --no-deps front
# Si quieres probar full local + dominio, usa:
# $COMPOSE up -d

echo ""
$COMPOSE ps
echo ""
echo "Front local: http://localhost:8086"
echo "Front público (Ingress k8s): https://ti3-brojas.dev.censei.cl"
echo "Si el dominio del Ingress cambia, edita $DEPLOY_ENV y re-ejecuta este script"
echo ""
echo "Test dominio:"
echo "  curl -v $API_BASE_URL/health"
echo "  curl -v $API_BASE_URL/health/db"
