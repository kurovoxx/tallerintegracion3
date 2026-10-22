#!/usr/bin/env bash
# Genera carga seleccionable (notes/social/academic) contra el cluster y
# muestra el efecto en las réplicas HPA. Ver .ade/Miguel/plans/20261017-feat-hpa-loadgen.md
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
NS="${NS:-student-brojas}"
QUOTA_NAME="${QUOTA_NAME:-student-provisioner-quota}"
MINT_SCRIPT="$SCRIPT_DIR/mint-load-token.py"
MONITOR_SCRIPT="$SCRIPT_DIR/monitor-traffic.sh"
IMAGE="busybox:1.36"
DRY_RUN="${LOAD_TEST_DRY_RUN:-0}"
PARALLEL="${LOAD_TEST_PARALLEL:-0}"

SELECTED=()
declare -A WORKERS=() DURATION=() DURATION_SEC=()
declare -A REQ_CPU=() REQ_MEM=() LIM_CPU=() LIM_MEM=()
declare -A REQ_CPU_M=() REQ_MEM_MI=() LIM_CPU_M=() LIM_MEM_MI=()
declare -A R_OK=() R_ERR=() R_UNAUTH=()
JOB_NAMES=()
declare -A JOB_CASE=()
declare -A PEAK=() HPA_SEEN=()
declare -A SVC_REQ_CPU=() SVC_REQ_MEM=() SVC_LIM_CPU=() SVC_LIM_MEM=() SVC_MAX=()
MON_PID=""
CLEANUP_FAILED=0
METRICS_ABORT=0

usage() {
  cat <<'EOF'
Uso: load-test.sh [--dry-run] [--namespace <ns>] [--parallel]

Genera 1 Job por caso elegido. Todos los prompts muestran un default y aceptan
Enter; un Enter tras otro ejecuta la demo completa (all secuencial, 40 workers,
60s). Con `all`, los casos se ejecutan de a uno por defecto para no exceder la
cuota; `--parallel` los lanza todos juntos (solo si la cuota lo permite).

Opciones:
  --dry-run            Valida la configuración sin tocar el cluster (pruebas)
  --namespace <ns>     Namespace (default: student-brojas)
  --parallel           Ejecuta todos los casos a la vez
  -h, --help           Muestra esta ayuda
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --namespace) NS="${2:?falta el valor de --namespace}"; shift 2 ;;
    --parallel) PARALLEL=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "opción desconocida: $1" >&2; usage >&2; exit 2 ;;
  esac
done

die() {
  echo "error: $*" >&2
  exit 1
}

ask() {
  local label="$1" default="$2" input=""
  printf '%s [%s]: ' "$label" "$default"
  if ! IFS= read -r input; then
    input=""
  fi
  REPLY="${input:-$default}"
}

parse_cases() {
  local raw="${1,,}" token
  raw="${raw//,/ }"
  local -A picked=()
  SELECTED=()
  for token in $raw; do
    case "$token" in
      all) picked[notes]=1; picked[social]=1; picked[academic]=1 ;;
      notes|social|academic) picked[$token]=1 ;;
      *) die "caso inválido: $token (usa notes, social, academic o all)" ;;
    esac
  done
  if [ "${#picked[@]}" -eq 0 ]; then
    die "selecciona al menos un caso (notes, social, academic o all)"
  fi
  for token in notes social academic; do
    if [ -n "${picked[$token]:-}" ]; then
      SELECTED+=("$token")
    fi
  done
}

normalize_duration() {
  local value="${1,,}"
  if [[ "$value" =~ ^([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^([0-9]+)s$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^([0-9]+)m$ ]]; then
    echo $((BASH_REMATCH[1] * 60))
  elif [[ "$value" =~ ^([0-9]+)h$ ]]; then
    echo $((BASH_REMATCH[1] * 3600))
  else
    return 1
  fi
}

cpu_millicores() {
  local value="${1,,}"
  if [[ "$value" =~ ^([0-9]+)m$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    awk -v x="$value" 'BEGIN { printf "%d", x * 1000 }'
  else
    return 1
  fi
}

mem_mebibytes() {
  local value="${1,,}"
  if [[ "$value" =~ ^[0-9]+([.][0-9]+)?(ki|mi|gi|k|m|g)?$ ]]; then
    awk -v x="$value" 'BEGIN {
      if (x ~ /ki$/) { sub(/ki$/, "", x); printf "%d", x * 1024 / 1048576 }
      else if (x ~ /mi$/) { sub(/mi$/, "", x); printf "%d", x }
      else if (x ~ /gi$/) { sub(/gi$/, "", x); printf "%d", x * 1024 }
      else if (x ~ /k$/) { sub(/k$/, "", x); printf "%d", x * 1000 / 1048576 }
      else if (x ~ /m$/) { sub(/m$/, "", x); printf "%d", x * 1000000 / 1048576 }
      else if (x ~ /g$/) { sub(/g$/, "", x); printf "%d", x * 1000000000 / 1048576 }
      else { printf "%d", x / 1048576 }
    }'
  else
    return 1
  fi
}

quota_get() {
  kubectl get resourcequota "$QUOTA_NAME" -n "$NS" -o "jsonpath=$1" 2>/dev/null || true
}

read_service_resources() {
  local svc req_cpu req_mem lim_cpu lim_mem max_replicas value
  for svc in notes social infacadem; do
    req_cpu=$(kubectl get deployment "$svc" -n "$NS" \
      -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}' 2>/dev/null || true)
    req_mem=$(kubectl get deployment "$svc" -n "$NS" \
      -o jsonpath='{.spec.template.spec.containers[0].resources.requests.memory}' 2>/dev/null || true)
    lim_cpu=$(kubectl get deployment "$svc" -n "$NS" \
      -o jsonpath='{.spec.template.spec.containers[0].resources.limits.cpu}' 2>/dev/null || true)
    lim_mem=$(kubectl get deployment "$svc" -n "$NS" \
      -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}' 2>/dev/null || true)
    max_replicas=$(kubectl get hpa "$svc" -n "$NS" \
      -o jsonpath='{.spec.maxReplicas}' 2>/dev/null || true)

    if [ -z "$req_cpu" ] || [ -z "$req_mem" ] || [ -z "$lim_cpu" ] || [ -z "$lim_mem" ]; then
      echo "error: no pude leer los resources de deployment/$svc en $NS" >&2
      return 1
    fi
    if ! [[ "$max_replicas" =~ ^[0-9]+$ ]]; then
      echo "error: no pude leer maxReplicas de hpa/$svc en $NS" >&2
      return 1
    fi

    if ! value="$(cpu_millicores "$req_cpu")"; then
      echo "error: request CPU inválido en deployment/$svc: $req_cpu" >&2
      return 1
    fi
    SVC_REQ_CPU[$svc]="$value"
    if ! value="$(mem_mebibytes "$req_mem")"; then
      echo "error: request memoria inválido en deployment/$svc: $req_mem" >&2
      return 1
    fi
    SVC_REQ_MEM[$svc]="$value"
    if ! value="$(cpu_millicores "$lim_cpu")"; then
      echo "error: limit CPU inválido en deployment/$svc: $lim_cpu" >&2
      return 1
    fi
    SVC_LIM_CPU[$svc]="$value"
    if ! value="$(mem_mebibytes "$lim_mem")"; then
      echo "error: limit memoria inválido en deployment/$svc: $lim_mem" >&2
      return 1
    fi
    SVC_LIM_MEM[$svc]="$value"
    SVC_MAX[$svc]="$max_replicas"
  done
  return 0
}

check_quota() {
  local hard_pods used_pods hard_req_cpu used_req_cpu hard_req_mem used_req_mem
  local hard_lim_cpu used_lim_cpu hard_lim_mem used_lim_mem
  hard_pods=$(quota_get '{.status.hard.pods}')
  used_pods=$(quota_get '{.status.used.pods}')
  hard_req_cpu=$(quota_get '{.status.hard.requests\.cpu}')
  used_req_cpu=$(quota_get '{.status.used.requests\.cpu}')
  hard_req_mem=$(quota_get '{.status.hard.requests\.memory}')
  used_req_mem=$(quota_get '{.status.used.requests\.memory}')
  hard_lim_cpu=$(quota_get '{.status.hard.limits\.cpu}')
  used_lim_cpu=$(quota_get '{.status.used.limits\.cpu}')
  hard_lim_mem=$(quota_get '{.status.hard.limits\.memory}')
  used_lim_mem=$(quota_get '{.status.used.limits\.memory}')

  if [ -z "$hard_pods" ] || [ -z "$used_pods" ] || [ -z "$hard_req_cpu" ] || \
     [ -z "$used_req_cpu" ] || [ -z "$hard_req_mem" ] || [ -z "$used_req_mem" ] || \
     [ -z "$hard_lim_cpu" ] || [ -z "$used_lim_cpu" ] || \
     [ -z "$hard_lim_mem" ] || [ -z "$used_lim_mem" ]; then
    echo "error: no pude leer la cuota $QUOTA_NAME en $NS" >&2
    echo "       revisa el contexto de kubectl y que la cuota exista" >&2
    return 1
  fi

  if ! [[ "$used_pods" =~ ^[0-9]+$ ]] || ! [[ "$hard_pods" =~ ^[0-9]+$ ]]; then
    echo "error: la cuota de pods no es numérica (hard=$hard_pods used=$used_pods)" >&2
    return 1
  fi
  used_req_cpu=$(cpu_millicores "$used_req_cpu") || { echo "error: used CPU inválido: $used_req_cpu" >&2; return 1; }
  used_req_mem=$(mem_mebibytes "$used_req_mem") || { echo "error: used memoria inválida: $used_req_mem" >&2; return 1; }
  used_lim_cpu=$(cpu_millicores "$used_lim_cpu") || { echo "error: used CPU límite inválido: $used_lim_cpu" >&2; return 1; }
  used_lim_mem=$(mem_mebibytes "$used_lim_mem") || { echo "error: used memoria límite inválida: $used_lim_mem" >&2; return 1; }

  read_service_resources || return 1

  local extra_pods=0 extra_req_cpu=0 extra_req_mem=0 extra_lim_cpu=0 extra_lim_mem=0
  local svc cur add
  for svc in notes social infacadem; do
    cur=$(kubectl get deployment "$svc" -n "$NS" -o jsonpath='{.status.replicas}' 2>/dev/null || true)
    [[ "$cur" =~ ^[0-9]+$ ]] || cur=0
    add=$((SVC_MAX[$svc] - cur))
    [ "$add" -lt 0 ] && add=0
    extra_pods=$((extra_pods + add))
    extra_req_cpu=$((extra_req_cpu + add * SVC_REQ_CPU[$svc]))
    extra_req_mem=$((extra_req_mem + add * SVC_REQ_MEM[$svc]))
    extra_lim_cpu=$((extra_lim_cpu + add * SVC_LIM_CPU[$svc]))
    extra_lim_mem=$((extra_lim_mem + add * SVC_LIM_MEM[$svc]))
  done

  local job_pods=0 job_req_cpu=0 job_req_mem=0 job_lim_cpu=0 job_lim_mem=0 case_name
  if [ "$PARALLEL" -eq 1 ]; then
    job_pods=${#SELECTED[@]}
    for case_name in "${SELECTED[@]}"; do
      job_req_cpu=$((job_req_cpu + REQ_CPU_M[$case_name]))
      job_req_mem=$((job_req_mem + REQ_MEM_MI[$case_name]))
      job_lim_cpu=$((job_lim_cpu + LIM_CPU_M[$case_name]))
      job_lim_mem=$((job_lim_mem + LIM_MEM_MI[$case_name]))
    done
  else
    job_pods=1
    for case_name in "${SELECTED[@]}"; do
      [ "${REQ_CPU_M[$case_name]}" -gt "$job_req_cpu" ] && job_req_cpu="${REQ_CPU_M[$case_name]}"
      [ "${REQ_MEM_MI[$case_name]}" -gt "$job_req_mem" ] && job_req_mem="${REQ_MEM_MI[$case_name]}"
      [ "${LIM_CPU_M[$case_name]}" -gt "$job_lim_cpu" ] && job_lim_cpu="${LIM_CPU_M[$case_name]}"
      [ "${LIM_MEM_MI[$case_name]}" -gt "$job_lim_mem" ] && job_lim_mem="${LIM_MEM_MI[$case_name]}"
    done
  fi

  local proj_pods=$((used_pods + extra_pods + job_pods))
  local proj_req_cpu=$((used_req_cpu + extra_req_cpu + job_req_cpu))
  local proj_req_mem=$((used_req_mem + extra_req_mem + job_req_mem))
  local proj_lim_cpu=$((used_lim_cpu + extra_lim_cpu + job_lim_cpu))
  local proj_lim_mem=$((used_lim_mem + extra_lim_mem + job_lim_mem))

  local hard_req_cpu_m hard_req_mem_mi hard_lim_cpu_m hard_lim_mem_mi
  hard_req_cpu_m=$(cpu_millicores "$hard_req_cpu") || { echo "error: cuota CPU inválida: $hard_req_cpu" >&2; return 1; }
  hard_req_mem_mi=$(mem_mebibytes "$hard_req_mem") || { echo "error: cuota memoria inválida: $hard_req_mem" >&2; return 1; }
  hard_lim_cpu_m=$(cpu_millicores "$hard_lim_cpu") || { echo "error: cuota CPU límite inválida: $hard_lim_cpu" >&2; return 1; }
  hard_lim_mem_mi=$(mem_mebibytes "$hard_lim_mem") || { echo "error: cuota memoria límite inválida: $hard_lim_mem" >&2; return 1; }

  if [ "$PARALLEL" -eq 1 ]; then
    echo "Cuota $QUOTA_NAME (paralelo): usado → proyectado / hard"
  else
    echo "Cuota $QUOTA_NAME (secuencial): usado → proyectado / hard"
  fi
  printf '  pods            %4s → %4s / %4s\n' "$used_pods" "$proj_pods" "$hard_pods"
  printf '  requests.cpu    %4sm → %4sm / %4sm\n' "$used_req_cpu" "$proj_req_cpu" "$hard_req_cpu_m"
  printf '  requests.memory %4sMi → %4sMi / %4sMi\n' "$used_req_mem" "$proj_req_mem" "$hard_req_mem_mi"
  printf '  limits.cpu      %4sm → %4sm / %4sm\n' "$used_lim_cpu" "$proj_lim_cpu" "$hard_lim_cpu_m"
  printf '  limits.memory   %4sMi → %4sMi / %4sMi\n' "$used_lim_mem" "$proj_lim_mem" "$hard_lim_mem_mi"

  local over=0
  if [ "$proj_pods" -gt "$hard_pods" ]; then
    echo "  ✗ pods exceden la cuota por $((proj_pods - hard_pods))" >&2
    over=1
  fi
  if [ "$proj_req_cpu" -gt "$hard_req_cpu_m" ]; then
    echo "  ✗ requests.cpu excede por $((proj_req_cpu - hard_req_cpu_m))m" >&2
    over=1
  fi
  if [ "$proj_req_mem" -gt "$hard_req_mem_mi" ]; then
    echo "  ✗ requests.memory excede por $((proj_req_mem - hard_req_mem_mi))Mi" >&2
    over=1
  fi
  if [ "$proj_lim_cpu" -gt "$hard_lim_cpu_m" ]; then
    echo "  ✗ limits.cpu excede por $((proj_lim_cpu - hard_lim_cpu_m))m" >&2
    over=1
  fi
  if [ "$proj_lim_mem" -gt "$hard_lim_mem_mi" ]; then
    echo "  ✗ limits.memory excede por $((proj_lim_mem - hard_lim_mem_mi))Mi" >&2
    over=1
  fi
  if [ "$over" -eq 1 ]; then
    if [ "$PARALLEL" -eq 1 ]; then
      echo "Prueba sin --parallel (secuencial) o baja resources; no se crean Jobs." >&2
    else
      echo "Baja resources o elige menos casos; no se crean Jobs." >&2
    fi
    return 1
  fi
  echo "  ✓ hay headroom para los Jobs seleccionados"
  return 0
}

verify_deployments() {
  local svc
  echo "Verificando rollout de notes, social e infacadem..."
  for svc in notes social infacadem; do
    if ! kubectl rollout status "deployment/$svc" -n "$NS" --timeout=30s >/dev/null 2>&1; then
      echo "error: deployment/$svc no está listo (rollout fallido o pods no disponibles)." >&2
      echo "       revisa k8s/manifests.yaml, el LimitRange (mín. 64Mi) y los eventos del namespace;" >&2
      echo "       no se crean Jobs hasta que los 3 servicios estén sanos." >&2
      return 1
    fi
  done
  echo "  ✓ los 3 servicios están listos"
  return 0
}

job_script() {
  case "$1" in
    academic)
      cat <<'ACADEMIC_SCRIPT'
#!/bin/sh
set -u
BASE="$TARGET"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
END=$(( $(date +%s) + DURATION ))
LAST_RC=0

request() {
  wget -S -O /dev/null -T 10 --header "Authorization: Bearer $TOKEN" "$1" >"$TMP/hdr" 2>&1
  LAST_RC=$?
  if grep -q 'HTTP/.* 401' "$TMP/hdr"; then
    echo "ERROR: 401 No autorizado en $1" >&2
    : > "$TMP/unauthorized"
  fi
}

fetch() {
  wget -S -O "$TMP/body" -T 10 --header "Authorization: Bearer $TOKEN" "$1" >"$TMP/hdr" 2>&1
  LAST_RC=$?
  if grep -q 'HTTP/.* 401' "$TMP/hdr"; then
    echo "ERROR: 401 No autorizado en $1" >&2
    : > "$TMP/unauthorized"
  fi
}

extract_ids() {
  grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' "$1" 2>/dev/null | sed 's/.*"\([^"]*\)"$/\1/'
}

SUB_IDS=""
fetch "$BASE/curricula"
if [ ! -f "$TMP/unauthorized" ] && [ "$LAST_RC" -eq 0 ]; then
  CUR=$(extract_ids "$TMP/body" | head -n 1)
  if [ -n "$CUR" ]; then
    fetch "$BASE/curricula/$CUR/subjects"
    if [ ! -f "$TMP/unauthorized" ] && [ "$LAST_RC" -eq 0 ]; then
      SUB_IDS=$(extract_ids "$TMP/body" | tr '\n' ' ')
    fi
  fi
fi

if [ -f "$TMP/unauthorized" ]; then
  echo "RESULT case=academic ok=0 err=0 unauthorized=1"
  exit 1
fi

worker() {
  i="$1"
  : > "$TMP/w$i.ok"
  : > "$TMP/w$i.err"
  while [ "$(date +%s)" -lt "$END" ]; do
    if [ -n "$SUB_IDS" ]; then
      for sid in $SUB_IDS; do
        [ "$(date +%s)" -ge "$END" ] && break
        request "$BASE/subjects/$sid/average"
        if [ "$LAST_RC" -eq 0 ] && [ ! -f "$TMP/unauthorized" ]; then
          echo ok >> "$TMP/w$i.ok"
        else
          echo err >> "$TMP/w$i.err"
        fi
        [ -f "$TMP/unauthorized" ] && break
      done
    else
      request "$BASE/academic/overview"
      if [ "$LAST_RC" -eq 0 ] && [ ! -f "$TMP/unauthorized" ]; then
        echo ok >> "$TMP/w$i.ok"
      else
        echo err >> "$TMP/w$i.err"
      fi
    fi
    [ -f "$TMP/unauthorized" ] && break
  done
}

i=1
while [ "$i" -le "$WORKERS" ]; do
  worker "$i" &
  i=$((i + 1))
done
wait

OK=$(cat "$TMP"/*.ok 2>/dev/null | wc -l)
ERR=$(cat "$TMP"/*.err 2>/dev/null | wc -l)
OK=$((OK))
ERR=$((ERR))
if [ -f "$TMP/unauthorized" ]; then
  echo "RESULT case=academic ok=$OK err=$ERR unauthorized=1"
  exit 1
fi
echo "RESULT case=academic ok=$OK err=$ERR unauthorized=0"
ACADEMIC_SCRIPT
      ;;
    *)
      cat <<'HTTP_SCRIPT'
#!/bin/sh
set -u
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
END=$(( $(date +%s) + DURATION ))
LAST_RC=0

request() {
  wget -S -O /dev/null -T 10 --header "Authorization: Bearer $TOKEN" "$1" >"$TMP/hdr" 2>&1
  LAST_RC=$?
  if grep -q 'HTTP/.* 401' "$TMP/hdr"; then
    echo "ERROR: 401 No autorizado en $1" >&2
    : > "$TMP/unauthorized"
  fi
}

worker() {
  i="$1"
  : > "$TMP/w$i.ok"
  : > "$TMP/w$i.err"
  while [ "$(date +%s)" -lt "$END" ]; do
    request "$TARGET"
    if [ "$LAST_RC" -eq 0 ] && [ ! -f "$TMP/unauthorized" ]; then
      echo ok >> "$TMP/w$i.ok"
    else
      echo err >> "$TMP/w$i.err"
    fi
    [ -f "$TMP/unauthorized" ] && break
  done
}

i=1
while [ "$i" -le "$WORKERS" ]; do
  worker "$i" &
  i=$((i + 1))
done
wait

OK=$(cat "$TMP"/*.ok 2>/dev/null | wc -l)
ERR=$(cat "$TMP"/*.err 2>/dev/null | wc -l)
OK=$((OK))
ERR=$((ERR))
if [ -f "$TMP/unauthorized" ]; then
  echo "RESULT case=$CASE ok=$OK err=$ERR unauthorized=1"
  exit 1
fi
echo "RESULT case=$CASE ok=$OK err=$ERR unauthorized=0"
HTTP_SCRIPT
      ;;
  esac
}

job_target() {
  case "$1" in
    notes) echo "http://notes:8082/notes/me" ;;
    social) echo "http://social:8083/groups/me" ;;
    academic) echo "http://academic:8081" ;;
  esac
}

JOB_NAME=""
job_manifest() {
  local name="$1" case_name="$2" duration="$3" workers="$4"
  local req_cpu="$5" req_mem="$6" lim_cpu="$7" lim_mem="$8"
  local target script
  target="$(job_target "$case_name")"
  script="$(job_script "$case_name")"
  cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: $name
  namespace: $NS
  labels: {app: loadgen, loadgen-case: $case_name}
spec:
  backoffLimit: 0
  activeDeadlineSeconds: $((duration + 180))
  ttlSecondsAfterFinished: 300
  template:
    metadata:
      labels: {app: loadgen, loadgen-case: $case_name}
    spec:
      restartPolicy: Never
      containers:
        - name: loadgen
          image: $IMAGE
          envFrom:
            - secretRef: {name: loadgen-token}
          env:
            - {name: CASE, value: "$case_name"}
            - {name: TARGET, value: "$target"}
            - {name: WORKERS, value: "$workers"}
            - {name: DURATION, value: "$duration"}
          resources:
            requests: {cpu: "$req_cpu", memory: "$req_mem"}
            limits: {cpu: "$lim_cpu", memory: "$lim_mem"}
          command: ["/bin/sh", "-c"]
          args:
            - |
$(printf '%s\n' "$script" | sed 's/^/              /')
EOF
}

sample_hpa() {
  local svc cur desired actual candidate value
  for svc in notes social infacadem; do
    cur=$(kubectl get hpa "$svc" -n "$NS" -o jsonpath='{.status.currentReplicas}' 2>/dev/null || true)
    desired=$(kubectl get hpa "$svc" -n "$NS" -o jsonpath='{.status.desiredReplicas}' 2>/dev/null || true)
    actual=$(kubectl get deployment "$svc" -n "$NS" -o jsonpath='{.status.replicas}' 2>/dev/null || true)
    candidate=0
    for value in "$cur" "$desired" "$actual"; do
      if [[ "$value" =~ ^[0-9]+$ ]] && [ "$value" -gt "$candidate" ]; then
        candidate="$value"
      fi
    done
    if [ "$candidate" -gt 0 ]; then
      HPA_SEEN[$svc]=1
    fi
    if [ "$candidate" -gt "${PEAK[$svc]:-0}" ]; then
      PEAK[$svc]="$candidate"
    fi
  done
}

start_case_job() {
  local case_name="$1" manifest
  JOB_NAME="loadgen-$case_name-$(date +%s)-$RANDOM"
  manifest="$(job_manifest "$JOB_NAME" "$case_name" "${DURATION_SEC[$case_name]}" \
    "${WORKERS[$case_name]}" "${REQ_CPU[$case_name]}" "${REQ_MEM[$case_name]}" \
    "${LIM_CPU[$case_name]}" "${LIM_MEM[$case_name]}")"
  printf '%s\n' "$manifest" | kubectl apply -f -
  JOB_NAMES+=("$JOB_NAME")
  JOB_CASE[$JOB_NAME]="$case_name"
}

wait_for_jobs() {
  local max_duration="$1"
  local deadline=$((SECONDS + max_duration + 180))
  local watch_start=$SECONDS metrics_checked=0
  local pending name succeeded failed svc state unknown
  echo "Esperando a que terminen los Jobs (máx. $((max_duration + 180))s)..."
  while :; do
    pending=0
    for name in "${JOB_NAMES[@]}"; do
      succeeded=$(kubectl get job "$name" -n "$NS" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)
      failed=$(kubectl get job "$name" -n "$NS" -o jsonpath='{.status.failed}' 2>/dev/null || true)
      if [ -z "$succeeded" ] && [ -z "$failed" ]; then
        pending=1
      fi
    done

    sample_hpa

    [ "$pending" -eq 0 ] && break
    if [ "$metrics_checked" -eq 0 ] && [ "$((SECONDS - watch_start))" -ge 30 ]; then
      metrics_checked=1
      unknown=""
      for svc in notes social infacadem; do
        state=$(kubectl get hpa "$svc" -n "$NS" \
          -o jsonpath='{.status.conditions[?(@.type=="ScalingActive")].status}' 2>/dev/null || true)
        if [ "$state" != "True" ]; then
          unknown="$unknown $svc"
        fi
      done
      if [ -n "$unknown" ]; then
        echo "aviso: HPA sin métricas válidas tras 30s:$unknown." >&2
        echo "       revisa metrics-server con el admin; se detiene la prueba." >&2
        METRICS_ABORT=1
        break
      fi
    fi
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "aviso: timeout esperando Jobs; se recolecta lo disponible" >&2
      break
    fi
    sleep 2
  done
}

post_job_sample() {
  local until=$((SECONDS + 20))
  while [ "$SECONDS" -lt "$until" ]; do
    sample_hpa
    sleep 2
  done
  sample_hpa
}

collect_job_result() {
  local name="$1" case_name line
  case_name="${JOB_CASE[$name]}"
  line=$(kubectl logs "job/$name" -n "$NS" --tail=-1 2>/dev/null | grep '^RESULT ' | tail -n 1 || true)
  if [ -z "$line" ]; then
    R_OK[$case_name]="-"
    R_ERR[$case_name]="-"
    R_UNAUTH[$case_name]="?"
  else
    R_OK[$case_name]=$(printf '%s\n' "$line" | sed -n 's/.*ok=\([0-9]*\).*/\1/p')
    R_ERR[$case_name]=$(printf '%s\n' "$line" | sed -n 's/.*err=\([0-9]*\).*/\1/p')
    R_UNAUTH[$case_name]=$(printf '%s\n' "$line" | sed -n 's/.*unauthorized=\([01]\).*/\1/p')
  fi
}

collect_results() {
  local name
  for name in "${JOB_NAMES[@]}"; do
    collect_job_result "$name"
  done
}

stop_monitor() {
  if [ -n "${MON_PID:-}" ]; then
    kill "$MON_PID" 2>/dev/null || true
    wait "$MON_PID" 2>/dev/null || true
    MON_PID=""
  fi
}

cleanup_jobs() {
  if [ "${#JOB_NAMES[@]}" -eq 0 ]; then
    return 0
  fi
  if ! kubectl delete job "${JOB_NAMES[@]}" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1; then
    CLEANUP_FAILED=1
    echo "aviso: no pude eliminar los Jobs (¿RBAC jobs/delete?); el TTL de 300s hará la limpieza." >&2
    return 1
  fi
  JOB_NAMES=()
  return 0
}

on_exit() {
  stop_monitor
  cleanup_jobs || true
}
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ---------------------------------------------------------------------------
# 1. Selección y configuración (todos los prompts aceptan Enter)
# ---------------------------------------------------------------------------
ask "Casos (notes,social,academic,all)" "all"
parse_cases "$REPLY"

for case_name in "${SELECTED[@]}"; do
  echo "--- Configuración del Job: $case_name ---"
  ask "  Workers" "40"
  WORKERS[$case_name]="$REPLY"
  ask "  Duración (ej. 60s, 2m)" "60s"
  DURATION[$case_name]="$REPLY"
  ask "  CPU request" "100m"
  REQ_CPU[$case_name]="$REPLY"
  ask "  Memoria request" "64Mi"
  REQ_MEM[$case_name]="$REPLY"
  ask "  CPU limit" "500m"
  LIM_CPU[$case_name]="$REPLY"
  ask "  Memoria limit" "128Mi"
  LIM_MEM[$case_name]="$REPLY"
done

ask "User ID del token" "24e120f9-d698-442c-999e-56b55737985a"
TOKEN_USER="$REPLY"
ask "Expiración del token (30m, 24h, 7d)" "24h"
TOKEN_EXP="$REPLY"
ask "¿Ejecutar monitor de tráfico? (s/n)" "s"
RUN_MONITOR="$REPLY"

[[ "$RUN_MONITOR" =~ ^[sSyY] ]] || RUN_MONITOR="n"

for case_name in "${SELECTED[@]}"; do
  if ! [[ "${WORKERS[$case_name]}" =~ ^[1-9][0-9]*$ ]]; then
    die "workers inválidos para $case_name: ${WORKERS[$case_name]}"
  fi
  if ! DURATION_SEC[$case_name]="$(normalize_duration "${DURATION[$case_name]}")"; then
    die "duración inválida para $case_name: ${DURATION[$case_name]} (usa 60s, 2m, 1h)"
  fi
  if [ "${DURATION_SEC[$case_name]}" -le 0 ]; then
    die "la duración de $case_name debe ser mayor que cero"
  fi
  if ! REQ_CPU_M[$case_name]="$(cpu_millicores "${REQ_CPU[$case_name]}")"; then
    die "CPU request inválida para $case_name: ${REQ_CPU[$case_name]}"
  fi
  if ! REQ_MEM_MI[$case_name]="$(mem_mebibytes "${REQ_MEM[$case_name]}")"; then
    die "memoria request inválida para $case_name: ${REQ_MEM[$case_name]}"
  fi
  if ! LIM_CPU_M[$case_name]="$(cpu_millicores "${LIM_CPU[$case_name]}")"; then
    die "CPU limit inválida para $case_name: ${LIM_CPU[$case_name]}"
  fi
  if ! LIM_MEM_MI[$case_name]="$(mem_mebibytes "${LIM_MEM[$case_name]}")"; then
    die "memoria limit inválida para $case_name: ${LIM_MEM[$case_name]}"
  fi
  if [ "${REQ_CPU_M[$case_name]}" -le 0 ] || [ "${REQ_MEM_MI[$case_name]}" -le 0 ] || \
     [ "${LIM_CPU_M[$case_name]}" -le 0 ] || [ "${LIM_MEM_MI[$case_name]}" -le 0 ]; then
    die "los resources de $case_name deben ser mayores que cero"
  fi
  if [ "${LIM_CPU_M[$case_name]}" -lt "${REQ_CPU_M[$case_name]}" ] || \
     [ "${LIM_MEM_MI[$case_name]}" -lt "${REQ_MEM_MI[$case_name]}" ]; then
    die "los limits de $case_name no pueden ser menores que sus requests"
  fi
done

echo
if [ "$PARALLEL" -eq 1 ]; then
  echo "Namespace: $NS | Cuota: $QUOTA_NAME | Casos: ${SELECTED[*]} | modo: paralelo"
else
  echo "Namespace: $NS | Cuota: $QUOTA_NAME | Casos: ${SELECTED[*]} | modo: secuencial"
fi
for case_name in "${SELECTED[@]}"; do
  echo "  $case_name: ${WORKERS[$case_name]} workers, ${DURATION_SEC[$case_name]}s, " \
       "req ${REQ_CPU[$case_name]}/${REQ_MEM[$case_name]}, " \
       "lim ${LIM_CPU[$case_name]}/${LIM_MEM[$case_name]}"
done
echo "  monitor de tráfico: $RUN_MONITOR"

if [ "$DRY_RUN" -eq 1 ]; then
  echo
  echo "DRY-RUN: se omiten cuota, token, Jobs y monitor; configuración válida."
  exit 0
fi

# ---------------------------------------------------------------------------
# 2. Dependencias, cuota y token
# ---------------------------------------------------------------------------
command -v kubectl >/dev/null 2>&1 || die "kubectl no está instalado o no está en PATH"
command -v python3 >/dev/null 2>&1 || die "python3 es necesario para acuñar el token"
[ -x "$MINT_SCRIPT" ] || die "no encuentro $MINT_SCRIPT"
[ -x "$MONITOR_SCRIPT" ] || die "no encuentro $MONITOR_SCRIPT"

echo
verify_deployments || die "rollout de servicios incompleto; no se crean Jobs"
echo
check_quota || die "la carga no cabe en la cuota; no se crean Jobs"

echo
"$MINT_SCRIPT" --namespace "$NS" --user-id "$TOKEN_USER" --exp "$TOKEN_EXP"

# ---------------------------------------------------------------------------
# 3. Monitor + Jobs
# ---------------------------------------------------------------------------
if [[ "$RUN_MONITOR" =~ ^[sSyY] ]]; then
  "$MONITOR_SCRIPT" --namespace "$NS" --interval 3 &
  MON_PID=$!
fi

for svc in notes social infacadem; do
  PEAK[$svc]=0
  HPA_SEEN[$svc]=0
done

if [ "$PARALLEL" -eq 1 ]; then
  for case_name in "${SELECTED[@]}"; do
    start_case_job "$case_name"
  done
  max_duration=0
  for case_name in "${SELECTED[@]}"; do
    if [ "${DURATION_SEC[$case_name]}" -gt "$max_duration" ]; then
      max_duration="${DURATION_SEC[$case_name]}"
    fi
  done
  wait_for_jobs "$max_duration"
  post_job_sample
  collect_results
else
  for case_name in "${SELECTED[@]}"; do
    echo
    echo "--- Ejecutando caso: $case_name ---"
    start_case_job "$case_name"
    wait_for_jobs "${DURATION_SEC[$case_name]}"
    post_job_sample
    collect_results
    cleanup_jobs || true
    if [ "$METRICS_ABORT" -eq 1 ] || [ "$CLEANUP_FAILED" -eq 1 ]; then
      break
    fi
  done
fi

# ---------------------------------------------------------------------------
# 4. Resumen antes de borrar los Jobs
# ---------------------------------------------------------------------------
stop_monitor
cleanup_jobs || true

echo
echo "===== RESUMEN ====="
printf '%-10s %10s %9s %5s\n' "CASO" "REQUESTS" "ERRORES" "401"
for case_name in "${SELECTED[@]}"; do
  printf '%-10s %10s %9s %5s\n' "$case_name" "${R_OK[$case_name]:--}" \
    "${R_ERR[$case_name]:--}" "${R_UNAUTH[$case_name]:--}"
done
echo
echo "Pico de réplicas (HPA):"
for svc in notes social infacadem; do
  if [ "${HPA_SEEN[$svc]}" -eq 1 ]; then
    printf '  %-10s %s\n' "$svc" "${PEAK[$svc]}"
  else
    printf '  %-10s -\n' "$svc"
  fi
done
echo
echo "Token: user_id=$TOKEN_USER, exp=$TOKEN_EXP (Secret loadgen-token)."
if [ "$CLEANUP_FAILED" -eq 1 ]; then
  echo "Jobs: no se pudieron eliminar todos; el TTL de 300s hará la limpieza (revisa RBAC jobs/delete)."
else
  echo "Jobs eliminados."
fi
if [ "$METRICS_ABORT" -eq 1 ]; then
  echo "Aviso: la prueba se detuvo antes de completar todos los casos por métricas HPA inválidas."
fi
echo "Si hubo 401, revisa JWT_SECRET/issuer/audience del token."
