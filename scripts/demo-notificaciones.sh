#!/bin/bash
# Demo 3_5_14 sin sacar JWT a mano: crea escenario y dice qué mirar en la app.
# Uso: ./scripts/demo-notificaciones.sh
# Opcional: AUTH_URL=http://... SOCIAL_URL=http://... NOTES_URL=http://...
set -euo pipefail

AUTH_URL="${AUTH_URL:-http://localhost:8085}"
SOCIAL_URL="${SOCIAL_URL:-http://localhost:8083}"
NOTES_URL="${NOTES_URL:-http://localhost:8082}"
PASS="Pass1234"
STAMP=$(date +%s)
A="demoa$STAMP@test.invalid"
B="demob$STAMP@test.invalid"

echo "== 1. Salud =="
for u in "$AUTH_URL/health" "$SOCIAL_URL/health" "$NOTES_URL/health"; do
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$u" || echo "000")
  echo "  $u -> $code"
  [ "$code" = "200" ] || { echo "Levanta el backend primero (./scripts/run-local.sh)"; exit 1; }
done

echo "== 2. ¿Backend con endpoints nuevos? =="
body=$(curl -s --max-time 5 "$SOCIAL_URL/groups/00000000-0000-0000-0000-000000000000/meetings" || true)
if echo "$body" | grep -q "page not found"; then
  echo "  STALE: el social corriendo NO tiene GET /groups/:id/meetings."
  echo "  Reconstruye: docker compose build social && docker compose up -d social"
  exit 1
fi
echo "  OK: la ruta existe (respuesta JSON del middleware)."

echo "== 3. Usuarios =="
for E in "$A" "$B"; do
  curl -s -o /dev/null "$AUTH_URL/auth/register" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$E\",\"password\":\"$PASS\"}" || true
done
TA=$(curl -s "$AUTH_URL/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$A\",\"password\":\"$PASS\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
TB=$(curl -s "$AUTH_URL/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$B\",\"password\":\"$PASS\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
echo "  tokens OK (quedan en variables, no los necesitas)."

echo "== 4. Grupo + join + reunión =="
GID=$(curl -s "$SOCIAL_URL/groups" -H "Authorization: Bearer $TA" \
  -H 'Content-Type: application/json' -d '{"name":"Demo Notif"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["group_id"])')
INV=$(curl -s "$SOCIAL_URL/groups/$GID" -H "Authorization: Bearer $TA" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["invite_token"])')
curl -s -o /dev/null "$SOCIAL_URL/groups/$GID/join" -H "Authorization: Bearer $TB" \
  -H 'Content-Type: application/json' -d "{\"invite_token\":\"$INV\"}"
FUTURE=$(date -u -d '+7 days' +%Y-%m-%dT15:00:00Z)
curl -s -o /dev/null "$SOCIAL_URL/groups/$GID/meetings" -H "Authorization: Bearer $TA" \
  -H 'Content-Type: application/json' \
  -d "{\"title\":\"Daily demo\",\"scheduled_at\":\"$FUTURE\"}"
echo "  grupo $GID con reunión agendada."

echo "== 5. Verificación API (lo que verá la app) =="
echo "  -- GET /groups/:id/meetings (usuario B):"
curl -s "$SOCIAL_URL/groups/$GID/meetings" -H "Authorization: Bearer $TB"
echo
echo "  -- GET /notifications/me (usuario B):"
curl -s "$SOCIAL_URL/notifications/me" -H "Authorization: Bearer $TB"
echo

echo
echo "== En la app =="
echo "  1. Inicia sesión como: $B / $PASS"
echo "  2. Abre el grupo 'Demo Notif' → pestaña PRÓXIMAS (debe salir 'Daily demo')."
echo "  3. Campana arriba (badge 1) → diálogo → toca la notificación (se marca leída, badge a 0)."
