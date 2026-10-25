#!/usr/bin/env python3
"""Contrato HTTP de Notes sobre Compose ti3-e2e real; Google permanece mock.

No admite bases remotas ni lee .env. Conserva cuentas/grupos sintéticos del run
en el volumen E2E y elimina solo las notas que crea, usando su JWT real.
"""
import concurrent.futures
import json
from pathlib import Path
import re
import subprocess
import urllib.error
import urllib.request
import uuid

BASES = {"auth": "http://127.0.0.1:18085", "notes": "http://127.0.0.1:18082",
         "social": "http://127.0.0.1:18083"}
PDF = b"%PDF-1.4\n% local E2E fixture\n%%EOF\n"
coverage = set()
checks = 0


def request(service, method, path, expected=200, token=None, body=None, headers=None, route=None):
    global checks
    hdr = dict(headers or {})
    if token:
        hdr["Authorization"] = "Bearer " + token
    if body is not None and not isinstance(body, bytes):
        body = json.dumps(body).encode()
        hdr["Content-Type"] = "application/json"
    req = urllib.request.Request(BASES[service] + path, data=body, headers=hdr, method=method)
    try:
        response = urllib.request.urlopen(req, timeout=60)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = response.read()
        status, response_headers = response.status, response.headers
    assert status == expected, f"{method} {path}: {status} != {expected}: {data[:700]!r}"
    checks += 1
    if service == "notes" and route:
        coverage.add((method, route))
    if data and "application/json" in response_headers.get("Content-Type", ""):
        return json.loads(data)
    return data


def register(label):
    body = {"email": f"sprint3-{uuid.uuid4().hex}@test.invalid", "password": "Sprint3-Test-Pass1!",
            "display_name": label}
    user = request("auth", "POST", "/auth/register", 201, body=body)
    login = request("auth", "POST", "/auth/login", body=body)
    return user["id"], login["access_token"]


def multipart():
    boundary = "notes-e2e-boundary"
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="clase.pdf"\r\n'
            'Content-Type: application/pdf\r\n\r\n').encode() + PDF + f"\r\n--{boundary}--\r\n".encode()
    return body, {"Content-Type": "multipart/form-data; boundary=" + boundary}


def sql(query):
    return subprocess.check_output(["docker", "exec", "ti3-e2e-postgres", "psql", "-U", "postgres",
                                    "-d", "postgres", "-At", "-c", query], text=True).strip()


def main():
    # Verificar pertenencia al proyecto antes de cualquier escritura.
    for service in ("postgres", "notes", "auth", "social"):
        data = json.loads(subprocess.check_output(["docker", "inspect", f"ti3-e2e-{service}"]))[0]
        assert data["Config"]["Labels"]["com.docker.compose.project"] == "ti3-e2e"
    assert request("notes", "GET", "/health", route="/health")["status"] == "UP"
    assert request("notes", "GET", "/health/drive", route="/health/drive")["storage_mode"] == "mock"
    source = (Path(__file__).resolve().parents[1] / "back/notes/cmd/server/main.go").read_text(encoding="utf-8")
    routes = set(re.findall(r'(?:r|protected)\.(GET|POST|PATCH|DELETE)\("([^"]+)"', source))
    assert len(routes) == 21, f"actualizar contrato para {len(routes)} rutas"
    for method, route in sorted(routes):
        if route.startswith("/health"):
            continue
        path = re.sub(r":[A-Za-z]+", "11111111-1111-4111-8111-111111111111", route)
        request("notes", method, path, 401)  # Auth antes de validación/body.
    print("PASS 19 rutas privadas rechazan ausencia de JWT; 2 health públicos", flush=True)

    owner_id, owner = register("Sprint 3 autor")
    _, reader = register("Sprint 3 lector externo")
    created = set()
    try:
        payload = {"title": "Álgebra E2E", "visibility": "private", "content": "# Contenido original\n"}
        key = "sprint3-" + uuid.uuid4().hex
        idem = {"X-Idempotency-Key": key}
        def create():
            return request("notes", "POST", "/notes", 201, owner, payload, idem, "/notes")["note_id"]
        note = create()
        created.add(note)
        assert create() == note
        request("notes", "POST", "/notes", 409, owner, dict(payload, title="otro payload"), idem)
        # Replays concurrentes de claim completado deben mantener identidad.
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            assert set(pool.map(lambda _: create(), range(4))) == {note}
        assert sql(f"SELECT count(*) FROM notes.notes WHERE user_id='{owner_id}'") == "1"
        print("PASS create/replay/conflicto/concurrencia: una fila PostgreSQL", flush=True)

        request("notes", "GET", f"/notes/{note}", 404, reader)
        detail = request("notes", "GET", f"/notes/{note}", token=owner, route="/notes/:id")
        assert detail["content"] == payload["content"] and detail["attachments"] == []
        access = request("notes", "GET", f"/notes/{note}/access", token=owner, route="/notes/:id/access")
        assert access["can_read"] and access["can_write"]
        updated = request("notes", "PATCH", f"/notes/{note}", token=owner,
                          body={"title": "Editada", "content": "# Edición", "version": detail["version"]}, route="/notes/:id")
        assert updated["version"] == detail["version"] + 1
        request("notes", "PATCH", f"/notes/{note}", 409, owner, {"title": "obsoleta", "version": detail["version"]})
        request("notes", "DELETE", f"/notes/{note}", 404, reader)

        binary, headers = multipart()
        attachment = request("notes", "POST", f"/notes/{note}/attachments", 201, owner, binary, headers,
                             "/notes/:id/attachments")
        aid = attachment["attachment_id"]
        got = request("notes", "GET", f"/notes/{note}/attachments/{aid}/content", token=owner,
                      route="/notes/:id/attachments/:attachmentId/content")
        assert got == PDF
        request("notes", "GET", f"/notes/{note}/attachments/{aid}/content", 404, reader)
        upload = request("notes", "POST", "/notes/upload", 201, owner, binary, headers, "/notes/upload")
        linked = request("notes", "POST", f"/notes/{note}/attachments", 201, owner,
                         {"external_file_id": upload["external_file_id"], "file_name": "clase.pdf", "file_type": "application/pdf"})
        assert linked["external_file_id"] == upload["external_file_id"]
        request("notes", "DELETE", f"/notes/{note}/attachments/{aid}", 204, owner,
                route="/notes/:id/attachments/:attachmentId")
        print("PASS lectura/IDOR/versión/adjuntos binarios y enlace externo", flush=True)

        request("notes", "POST", f"/notes/{note}/save", 201, owner, route="/notes/:id/save")
        request("notes", "POST", f"/notes/{note}/save", 409, owner)
        request("notes", "POST", f"/notes/{note}/like", 201, owner, route="/notes/:id/like")
        request("notes", "POST", f"/notes/{note}/like", 409, owner)
        request("notes", "DELETE", f"/notes/{note}/like", 204, owner, route="/notes/:id/like")
        request("notes", "DELETE", f"/notes/{note}/like", 204, owner)
        copied = request("notes", "POST", f"/notes/{note}/copy", 201, owner,
                         headers={"X-Idempotency-Key": "copy-" + key}, route="/notes/:id/copy")
        created.add(copied["note_id"])
        assert copied["forked_from_note_id"] == note and copied["note_id"] != note

        group = request("social", "POST", "/groups", 201, owner, {"name": "Sprint 3 E2E"})["group_id"]
        request("notes", "GET", f"/groups/{group}/notes", 403, reader)
        for mode in ("link", "restricted"):
            share = request("notes", "POST", f"/notes/{note}/share", 201, owner,
                            {"group_id": group, "access_mode": mode}, route="/notes/:id/share")
            notes = request("notes", "GET", f"/groups/{group}/notes", token=owner, route="/groups/:id/notes")
            assert note in {n["id"] for n in notes["notes"]}
            request("notes", "GET", f"/notes/{note}", 200 if mode == "link" else 404, reader)
            request("notes", "DELETE", f'/notes/shared/{share["shared_note_id"]}', 204, owner,
                    route="/notes/shared/:sharedNoteId")
        request("notes", "POST", f"/notes/{note}/share", 201, owner, {"group_id": group, "access_mode": "link"})
        request("notes", "POST", "/notes/unshare-all", 403, reader, {"user_id": owner_id, "group_id": group})
        request("notes", "POST", "/notes/unshare-all", 204, owner,
                {"user_id": owner_id, "group_id": group}, route="/notes/unshare-all")
        assert sql(f"SELECT count(*) FROM notes.shared_notes WHERE note_id='{note}'") == "0"
        # Restricted sin OAuth real valida ACL de aplicación; los permisos email
        # Google requieren las suites de contrato con directorio/Drive controlados.
        print("PASS save/like/copy/share/unshare/grupo/IDOR; Social gRPC real", flush=True)

        for i in range(51):
            n = request("notes", "POST", "/notes", 201, owner, dict(payload, title=f"Página {i}"))
            created.add(n["note_id"])
        seen, cursor, pages = set(), "", 0
        while True:
            page = request("notes", "GET", "/notes/me?limit=50" + ("&cursor=" + cursor if cursor else ""),
                           token=owner, route="/notes/me")
            ids = {n["id"] for n in page["notes"]}
            assert not seen.intersection(ids)
            seen.update(ids)
            pages += 1
            cursor = page["next_cursor"]
            if not cursor:
                break
            assert pages < 5
        assert seen == created and pages == 2
        # Paginación de notas compartidas del grupo (keyset admin/fecha/id):
        # antes el cursor solo usaba shared_at y omitía comparticiones.
        shared_ids = sorted(created - {note, copied["note_id"]})[:5]
        for nid in shared_ids:
            request("notes", "POST", f"/notes/{nid}/share", 201, owner, {"group_id": group, "access_mode": "link"})
        gseen, gcursor, gpages = [], "", 0
        while True:
            gpage = request("notes", "GET", f"/groups/{group}/notes?limit=2" + ("&cursor=" + gcursor if gcursor else ""),
                            token=owner, route="/groups/:id/notes")
            gseen.extend(n["id"] for n in gpage["notes"])
            gpages += 1
            gcursor = gpage["next_cursor"]
            if not gcursor:
                break
            assert gpages < 10
        assert len(gseen) == len(set(gseen)) and set(gseen) == set(shared_ids), (gseen, shared_ids)
        print(f"PASS notas de grupo paginadas: {len(gseen)} en {gpages} páginas, sin omisiones", flush=True)
        empty = request("notes", "GET", "/notes/me", token=reader)
        assert empty["notes"] == []
        summary = request("notes", "POST", "/notes/reconcile", token=owner, route="/notes/reconcile")
        assert summary["removed_notes"] == 0 and summary["removed_attachments"] == 0
        print("PASS paginación 53 notas/2 páginas, aislamiento y reconcile", flush=True)
    finally:
        for note in created:
            request("notes", "DELETE", f"/notes/{note}", 204, owner, route="/notes/:id")
            request("notes", "DELETE", f"/notes/{note}", 404, owner)
    assert sql(f"SELECT count(*) FROM notes.notes WHERE user_id='{owner_id}'") == "0"
    assert coverage == routes, f"rutas sin éxito E2E: {routes-coverage}"
    print(f"PASS {len(coverage)}/21 rutas con éxito; {checks} respuestas verificadas; PostgreSQL real, Drive mock")


if __name__ == "__main__":
    main()
