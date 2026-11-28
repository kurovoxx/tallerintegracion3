#!/usr/bin/env bash
# Empaqueta Sigma Academy (Flutter Linux) como .deb.
#
# Uso:
#   ./scripts/build-linux-deb.sh                     # compila y empaqueta
#   SKIP_BUILD=1 ./scripts/build-linux-deb.sh        # reutiliza el bundle ya compilado
#   VERSION=1.0.1+2 ./scripts/build-linux-deb.sh     # fuerza la versión Debian
#
# Requisitos: Flutter con soporte Linux y dpkg-deb.
#   Ubuntu/Debian: sudo apt install dpkg-dev
#   Fedora:        sudo dnf install dpkg
#
# Salida: dist/sigma-academy_<version>_<arch>.deb
set -euo pipefail
umask 022

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONT_DIR="$ROOT_DIR/front"

PKG_NAME="sigma-academy"
DISPLAY_NAME="Sigma Academy"
BINARY_NAME="taller_integracion_front" # nombre técnico que genera Flutter/CMake
APP_ID="cl.uct.taller_integracion_front"
ICON_SOURCE="$FRONT_DIR/linux/icons/sigma-academy.png"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist}"

# Arquitectura del paquete. Flutter usa x64/arm64 como carpeta de build.
case "$(uname -m)" in
  x86_64 | amd64)
    DEB_ARCH="amd64"
    FLUTTER_ARCH="x64"
    ;;
  aarch64 | arm64)
    DEB_ARCH="arm64"
    FLUTTER_ARCH="arm64"
    ;;
  *)
    echo "error: arquitectura no soportada: $(uname -m)" >&2
    exit 1
    ;;
esac

command -v flutter >/dev/null 2>&1 || {
  echo "error: no se encontró 'flutter' en PATH" >&2
  exit 1
}
command -v dpkg-deb >/dev/null 2>&1 || {
  echo "error: no se encontró 'dpkg-deb'. Ubuntu/Debian: sudo apt install dpkg-dev. Fedora: sudo dnf install dpkg" >&2
  exit 1
}
[ -f "$ICON_SOURCE" ] || {
  echo "error: falta el ícono $ICON_SOURCE" >&2
  exit 1
}

# Versión: pubspec.yaml (Flutter la muestra como 1.0.0+1). Se puede sobreescribir.
VERSION="${VERSION:-$(sed -n 's/^version:[[:space:]]*//p' "$FRONT_DIR/pubspec.yaml" | head -n 1)}"
VERSION="${VERSION%$'\r'}"
if [ -z "$VERSION" ]; then
  echo "error: no se pudo leer 'version:' de front/pubspec.yaml" >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^[0-9][A-Za-z0-9.+~:-]*$ ]]; then
  echo "error: versión inválida para Debian: $VERSION" >&2
  exit 1
fi

# Maintainer válido para el control. En CI se puede pasar MAINTAINER=...
CONTACT_EMAIL="$(git -C "$ROOT_DIR" config user.email 2>/dev/null || true)"
CONTACT_EMAIL="${CONTACT_EMAIL:-sigma-academy@example.com}"
MAINTAINER="${MAINTAINER:-$DISPLAY_NAME <$CONTACT_EMAIL>}"

BUNDLE_DIR="$FRONT_DIR/build/linux/$FLUTTER_ARCH/release/bundle"
STAGE_DIR="$FRONT_DIR/build/deb/$PKG_NAME"
OUT_FILE="$OUT_DIR/${PKG_NAME}_${VERSION}_${DEB_ARCH}.deb"

if [ "${SKIP_BUILD:-0}" != "1" ]; then
  echo ">> Compilando Flutter Linux (release, $FLUTTER_ARCH)"
  (cd "$FRONT_DIR" && flutter build linux --release)
fi

[ -x "$BUNDLE_DIR/$BINARY_NAME" ] || {
  echo "error: no existe $BUNDLE_DIR/$BINARY_NAME" >&2
  echo "       compilá primero con: cd front && flutter build linux --release" >&2
  exit 1
}

# glibc mínima real del bundle: evita instalar en distros donde no arrancaría.
GLIBC_MIN=""
if command -v objdump >/dev/null 2>&1; then
  GLIBC_MIN="$(
    for f in "$BUNDLE_DIR/$BINARY_NAME" "$BUNDLE_DIR"/lib/*.so; do
      [ -f "$f" ] && objdump -T "$f" 2>/dev/null || true
    done | grep -o 'GLIBC_[0-9][0-9.]*' | sed 's/^GLIBC_//' | sort -Vu | tail -n 1 || true
  )"
fi
LIBC_DEP="libc6"
if [ -n "$GLIBC_MIN" ]; then
  LIBC_DEP="libc6 (>= $GLIBC_MIN)"
fi

echo ">> Armando árbol del paquete $PKG_NAME $VERSION ($DEB_ARCH)"
rm -rf "$STAGE_DIR"
mkdir -p \
  "$STAGE_DIR/DEBIAN" \
  "$STAGE_DIR/opt/$PKG_NAME" \
  "$STAGE_DIR/usr/bin" \
  "$STAGE_DIR/usr/share/applications" \
  "$STAGE_DIR/usr/share/icons/hicolor/512x512/apps"

# Bundle completo de Flutter: ejecutable + lib/ (plugins/engine) + data/ (assets).
cp -a "$BUNDLE_DIR/." "$STAGE_DIR/opt/$PKG_NAME/"

# Acceso desde el menú/PATH sin duplicar el binario.
ln -s "/opt/$PKG_NAME/$BINARY_NAME" "$STAGE_DIR/usr/bin/$PKG_NAME"

cp "$ICON_SOURCE" "$STAGE_DIR/usr/share/icons/hicolor/512x512/apps/$PKG_NAME.png"

cat >"$STAGE_DIR/usr/share/applications/$APP_ID.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=$DISPLAY_NAME
Comment=Aplicación de escritorio para gestión académica y colaboración
Exec=/usr/bin/$PKG_NAME %U
Icon=$PKG_NAME
Terminal=false
Categories=Education;
StartupNotify=true
StartupWMClass=$APP_ID
EOF

INSTALLED_SIZE="$(du -sk "$STAGE_DIR/opt/$PKG_NAME" | cut -f1)"
cat >"$STAGE_DIR/DEBIAN/control" <<EOF
Package: $PKG_NAME
Version: $VERSION
Section: education
Priority: optional
Architecture: $DEB_ARCH
Maintainer: $MAINTAINER
Installed-Size: $INSTALLED_SIZE
Depends: $LIBC_DEP, libstdc++6, libgtk-3-0 | libgtk-3-0t64, libglib2.0-0 | libglib2.0-0t64, libblkid1, liblzma5
Description: $DISPLAY_NAME
 Aplicación de escritorio para gestión académica y colaboración.
EOF

# Refresca el menú y el caché de íconos al instalar/desinstalar. Best-effort:
# si las herramientas no están, el paquete igual queda instalado.
for script in postinst postrm; do
  cat >"$STAGE_DIR/DEBIAN/$script" <<'EOF'
#!/bin/sh
set -e
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi
exit 0
EOF
  chmod 755 "$STAGE_DIR/DEBIAN/$script"
done

mkdir -p "$OUT_DIR"
rm -f "$OUT_FILE"

# Permisos limpios: 755 para directorios/ejecutables y 644 para archivos.
chmod -R go-w "$STAGE_DIR"

dpkg-deb --build --root-owner-group "$STAGE_DIR" "$OUT_FILE" >/dev/null

echo "OK: $OUT_FILE"
echo "    $(du -h "$OUT_FILE" | cut -f1)"
