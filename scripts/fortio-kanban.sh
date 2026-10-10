#!/bin/bash
# Prueba de carga Kanban (solo lectura, sin bcrypt ni writes) con fortio in-cluster.
# - Target: GET /groups/:id/sprint-sheet (ListSprintTasks, logOp por request)
# - Auth: POST /auth/login -> access_token (Bearer para social)
# - Abre 1 terminal con logs de social y otra con el fortio load.
set -euo pipefail

NS="${NS:-student-brojas}"
AUTH_PUBLIC="${AUTH_PUBLIC:-https://ti3-brojas.dev.censei.cl/api-auth}"
# Defaults de prueba (override con env: EMAIL=... PASSWORD=... GROUP_ID=... ./script)
EMAIL_DEFAULT="nose@gmail.com"
PASSWORD_DEFAULT="Nose1234"
GROUP_DEFAULT="a0e376c1-a4e2-479a-841f-999a731e73db"

echo "=== Fortio Kanban (social) ==="
echo "NS=$NS  AUTH=$AUTH_PUBLIC"
echo ""

read -r -p "Email [$EMAIL_DEFAULT]: " EMAIL
read -r -s -p "Password [****]: " PASSWORD; echo
read -r -p "Group ID [$GROUP_DEFAULT]: " GROUP_ID
read -r -p "Usuarios concurrentes (-c) [4]: " CONCURRENCY
read -r -p "QPS [10]: " QPS
read -r -p "Duración [30s]: " DURATION

EMAIL="${EMAIL:-$EMAIL_DEFAULT}"
PASSWORD="${PASSWORD:-$PASSWORD_DEFAULT}"
GROUP_ID="${GROUP_ID:-$GROUP_DEFAULT}"

CONCURRENCY="${CONCURRENCY:-4}"
QPS="${QPS:-10}"
DURATION="${DURATION:-30s}"
# Normaliza duración: fortio -t exige unidad Go ("10s", no "10")
DURATION="$(echo "$DURATION" | tr -d '[:space:]')"
if [[ "$DURATION" =~ ^[0-9]+$ ]]; then DURATION="${DURATION}s"; fi
# Valida -c entero
if ! [[ "$CONCURRENCY" =~ ^[0-9]+$ ]]; then echo "-c debe ser entero (recibido: $CONCURRENCY)" >&2; exit 1; fi

if ! command -v kubectl >/dev/null; then echo "falta kubectl" >&2; exit 1; fi
if ! command -v curl >/dev/null; then echo "falta curl" >&2; exit 1; fi
if ! command -v jq >/dev/null; then echo "falta jq (sudo apt install jq)" >&2; exit 1; fi
if [[ -z "$EMAIL" || -z "$PASSWORD" || -z "$GROUP_ID" ]]; then echo "email/password/group-id requeridos" >&2; exit 1; fi

echo ""
echo "-> Login contra $AUTH_PUBLIC/auth/login ..."
# Ingress usa cert self-signed -> -k. No fallar por eso.
LOGIN_RESP="$(curl -sk --max-time 15 --connect-timeout 10 -X POST "$AUTH_PUBLIC/auth/login" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg e "$EMAIL" --arg p "$PASSWORD" '{email:$e,password:$p}')" || true)"
if [[ -z "$LOGIN_RESP" ]]; then
  echo "Login sin respuesta (timeout 15s). Prueba manual:" >&2
  echo "  curl -v --max-time 15 -X POST \"$AUTH_PUBLIC/auth/login\" -H 'Content-Type: application/json' -d '{\"email\":\"$EMAIL\",\"password\":\"***\"}'" >&2
  echo "  kubectl get pods -n $NS; kubectl logs deploy/auth -n $NS --tail=50" >&2
  echo "  Bypass ingress: kubectl port-forward svc/auth 8080:8080 -n $NS &  AUTH_PUBLIC=http://localhost:8080 ./scripts/fortio-kanban.sh" >&2
  exit 1
fi
# Login responde {access_token,...} directo (ver auth_handler.go:86)
TOKEN="$(echo "$LOGIN_RESP" | jq -r '.access_token // .data.access_token // empty')"
if [[ -z "$TOKEN" ]]; then
  echo "Login falló. Respuesta:" >&2
  echo "$LOGIN_RESP" | jq . >&2 || echo "$LOGIN_RESP" >&2
  echo "Revisa email/password o membresía." >&2
  exit 1
fi
echo "-> Token OK (${#TOKEN} chars, expira según JWT)."

# Endpoint Kanban solo-lectura: GET /groups/:id/sprint-sheet
# (back/social/cmd/server/main.go:206, handler ListSprintTasks con logOp, sin writes ni bcrypt)
TARGET="http://social:8083/groups/${GROUP_ID}/sprint-sheet"
POD="fortio-kanban-$(date +%s)"
OVERRIDES='{ "spec": { "containers": [ { "name": "'"${POD}"'", "image": "fortio/fortio", "resources": { "requests": { "cpu": "25m", "memory": "64Mi" }, "limits": { "cpu": "100m", "memory": "128Mi" } } } ] } }'

# Limpia pods fortio anteriores en Failed/Completed que estorban el quota
kubectl delete pod -n "$NS" -l run=fortio-test --ignore-not-found >/dev/null 2>&1 || true
kubectl delete pod fortio-test -n "$NS" --ignore-not-found >/dev/null 2>&1 || true

CMD_LOGS="kubectl logs -f deployment/social -n ${NS} --tail=20"
# NOTA: imagen fortio ya trae entrypoint 'fortio', se pasa solo 'load ...' (sin repetir 'fortio').
# strategic merge es clave: con merge default el --overrides reemplaza el array containers
# completo y borra los args (load...) que genera kubectl run -> fortio cae a modo server.
CMD_FORTIO="kubectl run ${POD} -n ${NS} --rm -i --restart=Never --image=fortio/fortio --override-type=strategic --overrides='${OVERRIDES}' -- load -c ${CONCURRENCY} -qps ${QPS} -t ${DURATION} -H \"Authorization: Bearer ${TOKEN}\" ${TARGET}"

open_term() {
  local title="$1"; local cmd="$2"
  local full="echo '=== ${title} ==='; echo '\$ ${cmd}'; echo ''; ${cmd}; echo ''; echo '[fin] presiona Enter...'; read -r _"
  if command -v gnome-terminal >/dev/null; then
    gnome-terminal --title="$title" -- bash -c "$full; exec bash" &
    return 0
  elif command -v konsole >/dev/null; then
    konsole -p tabtitle="$title" -e bash -c "$full; exec bash" &
    return 0
  elif command -v xfce4-terminal >/dev/null; then
    xfce4-terminal --title="$title" -e "bash -c \"$full; exec bash\"" &
    return 0
  elif command -v xterm >/dev/null; then
    xterm -T "$title" -e bash -c "$full; exec bash" &
    return 0
  fi
  return 1
}

echo ""
echo "Target kanban: $TARGET"
echo "Pod fortio: $POD (-c $CONCURRENCY, qps $QPS, -t $DURATION)"
echo ""

if open_term "social-logs ($NS)" "$CMD_LOGS"; then
  echo "-> terminal de logs abierta."
else
  echo "-> no se encontró terminal gráfica, corriendo logs en fondo:"
  echo "   $CMD_LOGS"
  bash -c "$CMD_LOGS" &
fi
sleep 1

if open_term "fortio-kanban (-c $CONCURRENCY)" "$CMD_FORTIO"; then
  echo "-> terminal fortio abierta."
  echo "Tip: filtra 200s con: kubectl logs -f deployment/social -n $NS | grep ListSprintTasks"
else
  echo "-> sin terminal gráfica, corriendo fortio aquí:"
  echo "   $CMD_FORTIO"
  eval "$CMD_FORTIO"
fi
