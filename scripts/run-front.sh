#!/bin/bash
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
FRONT_DIR="$PROJECT_ROOT/front"

echo "=== TI3 Frontend Runner ==="
echo "Detectando SO: $(uname -s)"
echo ""
echo "¿Quieres correr Flutter para Linux o Windows?"
echo "  1) linux         - nativo Linux contra Pillán"
echo "  2) windows       - nativo Windows contra Pillán (Windows + Visual Studio)"
echo "  3) web           - flutter run -d chrome contra Pillán"
echo "  4) docker        - docker compose up front (web via nginx http://localhost:8086)"
echo "  5) linux-deploy  - nativo Linux contra dominio del Ingress (front/.env.prod.example, sin auth local)"
echo "  6) windows-deploy - nativo Windows contra dominio del Ingress (solo Windows + Visual Studio, sin auth local)"
echo ""
read -p "Elige [1-6] (default 1): " CHOICE
CHOICE=${CHOICE:-1}

# Fedora deps check para linux
check_fedora_deps() {
  if command -v dnf >/dev/null 2>&1; then
    if ! rpm -q gtk3-devel >/dev/null 2>&1; then
      echo "Instalando deps Fedora..."
      sudo dnf install -y clang cmake ninja-build gtk3-devel pkg-config xz-devel
    fi
  elif command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update && sudo apt-get install -y clang cmake ninja-build pkg-config libgtk-3-dev
  fi
}

case "$CHOICE" in
  1|linux|Linux)
    echo ">> Linux nativo"
    check_fedora_deps
    if ! command -v flutter >/dev/null 2>&1; then
      echo "Flutter no encontrado. Instálalo: https://docs.flutter.dev/get-started/install/linux"
      exit 1
    fi
    cd "$FRONT_DIR"
    flutter pub get
    flutter run -d linux
    ;;
  2|windows|Windows)
    if [[ "$(uname -s)" != *"NT"* && "$(uname -s)" != *"MINGW"* ]]; then
      echo "Estás en $(uname -s) (Fedora/Linux). flutter build windows solo funciona en Windows."
      echo "Opciones:"
      echo "  a) Compilar via Docker Windows (no disponible en Linux)"
      echo "  b) Usar web: elige opción 3 o 4"
      read -p "¿Quieres intentar web en su lugar? [y/N]: " Y
      if [[ "$Y" == "y" ]]; then
        cd "$FRONT_DIR" && flutter pub get && flutter run -d chrome
      else
        echo "Cancelado. En Windows nativo ejecuta: flutter run -d windows"
      fi
      exit 0
    fi
    cd "$FRONT_DIR"
    flutter pub get
    flutter run -d windows
    ;;
  3|web|Web)
    echo ">> Web (Chrome)"
    if ! command -v flutter >/dev/null 2>&1; then echo "Flutter no encontrado"; exit 1; fi
    cd "$FRONT_DIR"
    flutter pub get
    flutter run -d chrome --web-port 8085
    ;;
  4|docker|Docker)
    echo ">> Docker front (http://localhost:8086 via nginx /api-auth -> auth:8080)"
    cd "$PROJECT_ROOT"
    docker compose up --build front
    ;;
  5|linux-deploy|deploy)
    echo ">> Linux nativo contra dominio del Ingress (sin auth local)"
    echo "   Certificado de Pillán verificado mediante su huella SHA-256"
    check_fedora_deps
    if ! command -v flutter >/dev/null 2>&1; then echo "Flutter no encontrado"; exit 1; fi
    # Lee dominio de .env.prod o front/.env.prod.example
    DEPLOY_ENV="$PROJECT_ROOT/.env.prod"
    if [ ! -f "$DEPLOY_ENV" ]; then DEPLOY_ENV="$PROJECT_ROOT/front/.env.prod.example"; fi
    if [ ! -f "$DEPLOY_ENV" ]; then echo "No se encontró .env.prod ni front/.env.prod.example"; exit 1; fi
    # shellcheck disable=SC1090
    set -a; source "$DEPLOY_ENV"; set +a
    API_URL="${API_BASE_URL:-https://ti3-brojas.dev.censei.cl/api-auth}"
    NOTES_URL="${NOTES_BASE_URL:-https://ti3-brojas.dev.censei.cl/api-notes}"
    SOCIAL_URL="${SOCIAL_BASE_URL:-https://ti3-brojas.dev.censei.cl/api-social}"
    echo "API: $API_URL"
    cd "$FRONT_DIR"
    flutter pub get
    flutter run -d linux --dart-define=API_BASE_URL="$API_URL" --dart-define=NOTES_BASE_URL="$NOTES_URL" --dart-define=SOCIAL_BASE_URL="$SOCIAL_URL"
    ;;
  6|windows-deploy|wdeploy|windows_deploy)
    echo ">> Windows nativo contra dominio del Ingress (sin auth local)"
    echo "   Certificado de Pillán verificado mediante su huella SHA-256"
    if [[ "$(uname -s)" != *"NT"* && "$(uname -s)" != *"MINGW"* && "$(uname -s)" != *"MSYS"* ]]; then
      echo "Estás en $(uname -s) (Linux). flutter run -d windows solo funciona en Windows."
      echo "En este equipo usa opción 5 (linux-deploy). En Windows nativo elige 6."
      exit 1
    fi
    if ! command -v flutter >/dev/null 2>&1; then echo "Flutter no encontrado"; exit 1; fi
    # Lee dominio de .env.prod o front/.env.prod.example (igual que opción 5)
    DEPLOY_ENV="$PROJECT_ROOT/.env.prod"
    if [ ! -f "$DEPLOY_ENV" ]; then DEPLOY_ENV="$PROJECT_ROOT/front/.env.prod.example"; fi
    if [ ! -f "$DEPLOY_ENV" ]; then echo "No se encontró .env.prod ni front/.env.prod.example"; exit 1; fi
    # shellcheck disable=SC1090
    set -a; source "$DEPLOY_ENV"; set +a
    API_URL="${API_BASE_URL:-https://ti3-brojas.dev.censei.cl/api-auth}"
    NOTES_URL="${NOTES_BASE_URL:-https://ti3-brojas.dev.censei.cl/api-notes}"
    SOCIAL_URL="${SOCIAL_BASE_URL:-https://ti3-brojas.dev.censei.cl/api-social}"
    echo "API: $API_URL"
    cd "$FRONT_DIR"
    flutter pub get
    flutter run -d windows --dart-define=API_BASE_URL="$API_URL" --dart-define=NOTES_BASE_URL="$NOTES_URL" --dart-define=SOCIAL_BASE_URL="$SOCIAL_URL"
    ;;
  *)
    echo "Opción inválida"
    exit 1
    ;;
esac
