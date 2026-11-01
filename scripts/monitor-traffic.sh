#!/usr/bin/env bash
# Monitor de tráfico de notes/social/infacadem leyendo sus logs GIN.
# No crea pods: solo consulta pods, HPA y logs del cluster.
set -uo pipefail

NS="${NS:-student-brojas}"
INTERVAL=3
SINCE_TIME=""
ONCE=0
SERVICES=(notes social infacadem)

usage() {
  cat <<'EOF'
Uso: monitor-traffic.sh [opciones]

Opciones:
  --namespace <ns>    Namespace (default: student-brojas)
  --interval <seg>    Segundos entre muestras (default: 3)
  --since-time <ts>   RFC3339 desde donde leer logs (default: ahora)
  --once              Imprime una sola muestra y sale
  -h, --help          Muestra esta ayuda

Columnas: req/s, 2xx, 3xx, 4xx, 5xx, latencia media, p95, pods actuales y réplicas HPA.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --namespace) NS="${2:?falta el valor de --namespace}"; shift 2 ;;
    --interval) INTERVAL="${2:?falta el valor de --interval}"; shift 2 ;;
    --since-time) SINCE_TIME="${2:?falta el valor de --since-time}"; shift 2 ;;
    --once) ONCE=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "opción desconocida: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if ! [[ "$INTERVAL" =~ ^[0-9]+$ ]] || [ "$INTERVAL" -le 0 ]; then
  echo "error: --interval debe ser un entero mayor que cero" >&2
  exit 2
fi
if [ -n "$SINCE_TIME" ] && ! [[ "$SINCE_TIME" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$ ]]; then
  echo "error: --since-time debe ser RFC3339 (ej. 2026-10-21T04:00:00Z)" >&2
  exit 2
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "error: kubectl no está instalado o no está en PATH" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/monitor-traffic.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

declare -A CURSOR
declare -A APP_LOG_FAIL
LOG_WARNED=0
INITIAL_CURSOR="${SINCE_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%S.000000000Z")}"
for svc in "${SERVICES[@]}"; do
  CURSOR[$svc]="$INITIAL_CURSOR"
done

# Extrae métricas de una app. Deja en METRICS: total 2xx 4xx 5xx cursor.
collect_app() {
  local svc="$1"
  local app="$1"
  local logfile="$TMP_DIR/log.$svc"
  local latfile="$TMP_DIR/lat.$svc"
  local outfile="$TMP_DIR/out.$svc"
  local prev="${CURSOR[$svc]}"

  local log_err="$TMP_DIR/err.$svc"
  local rc=0
  : > "$latfile"
  kubectl logs -n "$NS" -l "app=$app" --timestamps=true \
    --since-time="$prev" --tail=-1 >"$logfile" 2>"$log_err"
  rc=$?
  if [ "$rc" -ne 0 ] && [ ! -s "$logfile" ]; then
    APP_LOG_FAIL[$svc]=1
  else
    APP_LOG_FAIL[$svc]=0
  fi

  awk -v latfile="$latfile" -v outfile="$outfile" -v prev="$prev" '
    BEGIN { maxts = prev }
    function norm_ts(ts,  base, frac) {
      if (ts == "") return ""
      base = substr(ts, 1, 19)
      frac = ""
      if (index(ts, ".") > 0) {
        frac = substr(ts, index(ts, ".") + 1)
        sub(/Z$/, "", frac)
      }
      while (length(frac) < 9) frac = frac "0"
      return base "." substr(frac, 1, 9) "Z"
    }
    function to_ms(l) {
      if (l ~ /ns$/) { sub(/ns$/, "", l); return (l + 0) / 1000000 }
      if (l ~ /µs$/ || l ~ /us$/) { sub(/(µs|us)$/, "", l); return (l + 0) / 1000 }
      if (l ~ /ms$/) { sub(/ms$/, "", l); return l + 0 }
      if (l ~ /s$/) { sub(/s$/, "", l); return (l + 0) * 1000 }
      return 0
    }
    {
      ts = ""
      for (i = 1; i <= 3; i++) {
        if ($i ~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T.*Z$/) { ts = $i; break }
      }
      tsn = norm_ts(ts)
      if (tsn != "" && tsn > maxts) maxts = tsn
      if (index($0, "[GIN]") == 0) next
      if (prev != "" && tsn != "" && tsn <= norm_ts(prev)) next

      n = split($0, parts, "|")
      if (n < 4) next
      status = parts[2]
      gsub(/^[ \t]+|[ \t]+$/, "", status)
      latency = parts[3]
      gsub(/^[ \t]+|[ \t]+$/, "", latency)

      total++
      if (status ~ /^2/) c2++
      else if (status ~ /^3/) c3++
      else if (status ~ /^4/) c4++
      else if (status ~ /^5/) c5++
      print to_ms(latency) >> latfile
    }
    END {
      if (maxts == "") maxts = prev
      printf "%d %d %d %d %d %s\n", total, c2, c3, c4, c5, maxts > outfile
      close(outfile)
    }
  ' "$logfile"

  TOTAL=0; C2=0; C3=0; C4=0; C5=0; NEW_CURSOR="$prev"
  if [ -s "$outfile" ]; then
    read -r TOTAL C2 C3 C4 C5 NEW_CURSOR < "$outfile"
    CURSOR[$svc]="$NEW_CURSOR"
  fi
  P95=0
  if [ "${TOTAL:-0}" -gt 0 ] && [ -s "$latfile" ]; then
    P95=$(sort -n "$latfile" | awk -v n="$TOTAL" '
      BEGIN { idx = int((n * 95 + 99) / 100); if (idx < 1) idx = 1 }
      NR == idx { print; exit }
    ')
  fi
}

print_snapshot() {
  local now
  now=$(date -u +"%H:%M:%S")
  echo
  echo "[$now UTC] ventana ${INTERVAL}s (logs GIN)"
  printf "%-10s %7s %5s %5s %5s %5s %9s %8s %5s %7s\n" \
    "SERVICIO" "REQ/S" "2xx" "3xx" "4xx" "5xx" "AVG(ms)" "P95(ms)" "PODS" "HPA"
  for svc in "${SERVICES[@]}"; do
    collect_app "$svc"
    local rate avg pods hpa
    rate=$(awk -v t="${TOTAL:-0}" -v i="$INTERVAL" 'BEGIN { printf "%.1f", t / i }')
    if [ "${TOTAL:-0}" -gt 0 ]; then
      avg=$(awk -v n="$TOTAL" -v file="$TMP_DIR/lat.$svc" 'BEGIN { }
        { sum += $1; count++ }
        END { if (count) printf "%.1f", sum / count; else printf "0.0" }
      ' "$TMP_DIR/lat.$svc")
    else
      avg="0.0"
    fi
    pods=$(kubectl get pods -n "$NS" -l "app=$svc" --no-headers 2>/dev/null | wc -l)
    pods=${pods//[[:space:]]/}
    hpa=$(kubectl get hpa "$svc" -n "$NS" \
      -o jsonpath='{.status.currentReplicas}/{.status.desiredReplicas}' 2>/dev/null || true)
    [ -n "$hpa" ] || hpa="-"
    printf "%-10s %7s %5s %5s %5s %5s %9s %8s %5s %7s\n" \
      "$svc" "$rate" "${C2:-0}" "${C3:-0}" "${C4:-0}" "${C5:-0}" "$avg" "${P95:-0}" "$pods" "$hpa"
  done
  local all_fail=1
  for svc in "${SERVICES[@]}"; do
    if [ "${APP_LOG_FAIL[$svc]:-0}" -eq 0 ]; then
      all_fail=0
    fi
  done
  if [ "$all_fail" -eq 1 ] && [ "$LOG_WARNED" -eq 0 ]; then
    echo "aviso: kubectl logs falló para todos los pods; revisa RBAC pods/log y --since-time." >&2
    LOG_WARNED=1
  fi
  echo "Cursor logs: ${CURSOR[notes]} | ${CURSOR[social]} | ${CURSOR[infacadem]}"
}

if [ "$ONCE" -eq 1 ]; then
  print_snapshot
  exit 0
fi

echo "Monitor de tráfico en namespace $NS (Ctrl-C para salir)."
while true; do
  print_snapshot
  sleep "$INTERVAL"
done
