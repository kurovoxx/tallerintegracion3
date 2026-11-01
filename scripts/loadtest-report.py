#!/usr/bin/env python3
"""Renderiza el reporte markdown de scripts/load-test.sh."""
import argparse
import datetime

SERVICES = ("notes", "social", "infacadem")


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--meta", required=True)
    parser.add_argument("--hpa", required=True)
    parser.add_argument("--pods", required=True)
    parser.add_argument("--monitor", default="")
    parser.add_argument("--output", required=True)
    return parser.parse_args()


def read_meta(path):
    keys = {"case": "cases", "result": "results", "hpa": "hpas", "service": "services"}
    meta = {"cases": [], "results": [], "hpas": [], "services": []}
    try:
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                line = line.rstrip("\n")
                if not line or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                if key in keys:
                    meta[keys[key]].append(value.split("|"))
                else:
                    meta[key] = value
    except FileNotFoundError:
        pass
    return meta


def read_hpa(path):
    rows = {}
    try:
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                parts = line.rstrip("\n").split("\t")
                if len(parts) < 5:
                    continue
                sample, service, current, desired, cpu = parts[:5]
                rows.setdefault(sample, {})[service] = (current, desired, cpu)
    except FileNotFoundError:
        pass
    return rows


def read_pods(path):
    pods = {}
    try:
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                parts = line.rstrip("\n").split("\t")
                if len(parts) < 9:
                    continue
                sample, app, name, created, ready, ready_ts, phase, node, restarts = parts[:9]
                pods[name] = {
                    "app": app,
                    "created": created,
                    "ready": ready,
                    "ready_ts": ready_ts,
                    "phase": phase,
                    "node": node,
                    "restarts": restarts,
                    "first_seen": sample,
                }
    except FileNotFoundError:
        pass
    return pods


def parse_ts(value):
    if not value or value == "-":
        return None
    try:
        return datetime.datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(
            tzinfo=datetime.timezone.utc
        )
    except ValueError:
        return None


def startup_seconds(created, ready_ts, ready):
    if ready != "True":
        return "-"
    start, end = parse_ts(created), parse_ts(ready_ts)
    if not start or not end or end < start:
        return "-"
    return str(int((end - start).total_seconds()))


def render_timeline(rows):
    lines = []
    times = sorted(rows)
    if not times:
        return lines
    lines.append(f"| Hora (UTC) | {' | '.join(f'{svc} réplicas (CPU)' for svc in SERVICES)} |")
    lines.append("|------------|" + "|".join("------" for _ in SERVICES) + "|")
    previous = None
    for index, sample in enumerate(times):
        values = rows[sample]
        key = tuple(values.get(svc) for svc in SERVICES)
        if key == previous and index != len(times) - 1:
            continue
        cells = []
        for svc in SERVICES:
            value = values.get(svc)
            if not value:
                cells.append("-")
                continue
            current, desired, cpu = value
            cpu_text = f"{cpu}%" if cpu.isdigit() else cpu
            cells.append(f"{current}/{desired} ({cpu_text})")
        lines.append(f"| {sample} | " + " | ".join(cells) + " |")
        previous = key
    return lines


def render_pods(pods):
    if not pods:
        return ["Sin muestras de pods."]
    lines = [
        "| Pod | app | Creado | Ready | Transición Ready | Arranque (s) | Fase final | Nodo | Restarts |",
        "|-----|-----|--------|-------|------------------|--------------|------------|------|----------|",
    ]
    for name, pod in sorted(pods.items(), key=lambda item: item[1]["created"]):
        lines.append(
            "| `{}` | {} | {} | {} | {} | {} | {} | {} | {} |".format(
                name,
                pod["app"],
                pod["created"],
                pod["ready"],
                pod["ready_ts"],
                startup_seconds(pod["created"], pod["ready_ts"], pod["ready"]),
                pod["phase"],
                pod["node"],
                pod["restarts"],
            )
        )
    return lines


def render(meta, hpa_rows, pods, monitor_text):
    lines = ["# Reporte de load test", ""]
    lines.append(f"- **Inicio (UTC):** {meta.get('start', '-')}")
    lines.append(f"- **Fin (UTC):** {meta.get('end', '-')}")
    lines.append(f"- **Duración total:** {meta.get('elapsed', '-')} s")
    lines.append(f"- **Namespace:** `{meta.get('namespace', '-')}`")
    lines.append(f"- **Contexto kubectl:** `{meta.get('context', '-')}`")
    lines.append(f"- **Modo:** {meta.get('mode', '-')}")
    lines.append(f"- **Muestreo de pods/HPA:** cada {meta.get('sample_interval', '-')} s")
    lines.append(f"- **`kubectl top`:** {meta.get('top_status', '-')}")
    lines.append("")

    lines.extend(
        [
            "## Configuración de la carga",
            "",
            "| Caso | Workers | Duración (s) | CPU req | Mem req | CPU lim | Mem lim |",
            "|------|---------|--------------|---------|---------|---------|---------|",
        ]
    )
    for row in meta["cases"]:
        if len(row) >= 7:
            lines.append("| " + " | ".join(row[:7]) + " |")
    lines.append("")

    lines.extend(
        [
            "## Resultados por caso",
            "",
            "| Caso | Requests OK | Errores | 401 |",
            "|------|-------------|---------|-----|",
        ]
    )
    for row in meta["results"]:
        if len(row) >= 4:
            lines.append("| " + " | ".join(row[:4]) + " |")
    lines.append("")

    lines.extend(
        [
            "## HPA",
            "",
            "| Servicio | min | max | Target CPU | Pico réplicas | Réplicas finales | CPU final |",
            "|----------|-----|-----|------------|---------------|------------------|-----------|",
        ]
    )
    for row in meta["hpas"]:
        if len(row) < 8:
            continue
        service, minimum, maximum, target, peak, current, desired, cpu = row[:8]
        cpu_text = f"{cpu}%" if cpu.isdigit() else cpu
        lines.append(
            f"| {service} | {minimum} | {maximum} | {target}% | {peak} | {current}/{desired} | {cpu_text} |"
        )
    lines.append("")

    timeline = render_timeline(hpa_rows)
    if timeline:
        lines.append(
            f"<details><summary>Timeline de réplicas y CPU (cada {meta.get('sample_interval', '?')} s)</summary>"
        )
        lines.append("")
        lines.extend(timeline)
        lines.append("")
        lines.append("</details>")
        lines.append("")

    lines.extend(["## Ciclo de vida de los pods", ""])
    lines.extend(render_pods(pods))
    lines.append("")

    lines.extend(
        [
            "## Recursos de los servicios",
            "",
            "| Servicio | CPU req | Mem req | CPU lim | Mem lim |",
            "|----------|---------|---------|---------|---------|",
        ]
    )
    for row in meta["services"]:
        if len(row) >= 5:
            lines.append("| " + " | ".join(row[:5]) + " |")
    lines.append("")

    lines.extend(["## Tráfico (logs GIN, ventana completa)", ""])
    if monitor_text.strip():
        lines.extend(["```text", monitor_text.rstrip(), "```"])
    else:
        lines.append("Sin datos de tráfico (monitor deshabilitado o sin logs en la ventana).")
    lines.append("")

    abort = "sí" if meta.get("metrics_abort") == "1" else "no"
    lines.append(f"- **Prueba abortada por métricas HPA:** {abort}")
    lines.append(f"- **Limpieza de Jobs:** {meta.get('cleanup', '-')}")
    lines.append("")
    return "\n".join(lines)


def main():
    args = parse_args()
    meta = read_meta(args.meta)
    hpa_rows = read_hpa(args.hpa)
    pods = read_pods(args.pods)
    monitor_text = ""
    if args.monitor:
        try:
            with open(args.monitor, encoding="utf-8") as fh:
                monitor_text = fh.read()
        except FileNotFoundError:
            pass
    report = render(meta, hpa_rows, pods, monitor_text)
    with open(args.output, "w", encoding="utf-8") as fh:
        fh.write(report)


if __name__ == "__main__":
    main()
