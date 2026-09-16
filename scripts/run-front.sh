#!/bin/bash
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
FRONT_DIR="$PROJECT_ROOT/front"

echo "=== TI3 Frontend Runner ==="
echo "Detectando SO: $(uname -s)"
echo ""
echo "¿Quieres correr Flutter para Linux o Windows?"
echo "  1) linux  - build y ejecuta nativo Linux (requiere Fedora: clang cmake ninja-build gtk3-devel)"
echo "  2) windows - build Windows (solo funciona si estás en Windows con Flutter + Visual Studio)"
echo "  3) web     - flutter run -d chrome"
echo "  4) docker  - docker compose up front (web via nginx en http://localhost:8085)"
echo ""
read -p "Elige [1-4] (default 1): " CHOICE
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
    echo ">> Docker front (http://localhost:8085)"
    cd "$PROJECT_ROOT"
    docker compose up --build front
    ;;
  *)
    echo "Opción inválida"
    exit 1
    ;;
esac
