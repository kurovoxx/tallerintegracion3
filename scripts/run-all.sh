#!/bin/bash
set -e
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

echo "=== TI3 Docker Compose ==="
echo "Levanta: postgres (5432) + auth (8080) + notes (8082) + social (8083) + front (8085)"
echo ""
echo "¿Levantar todo con docker compose? [y/N]: "
read -r GO
if [[ "$GO" != "y" && "$GO" != "Y" ]]; then
  echo "Cancelado. Para levantar manual:"
  echo "  docker compose up --build"
  echo "  ./scripts/run-front.sh   # para elegir linux/windows"
  exit 0
fi

docker compose up --build -d
echo ""
echo "Servicios:"
docker compose ps
echo ""
echo "Logs: docker compose logs -f"
echo "Front: http://localhost:8085 (web) | para nativo: ./scripts/run-front.sh"
echo "Auth health: curl http://localhost:8080/health"
echo "Notes health: curl http://localhost:8082/health"
echo "Social health: curl http://localhost:8083/health"
