#!/usr/bin/env python3
"""E2E del borrado de cuenta (3_2_16) sobre Compose ti3-e2e real.

Identity -> Notes -> Social por gRPC con PostgreSQL local; Drive y Stream mock.
Solo usa cuentas sintéticas (@test.invalid) creadas en este run. No lee .env ni
admite bases remotas: antes de escribir verifica que los contenedores sean del
proyecto Compose ti3-e2e.

Uso:
  docker compose -f docker-compose.e2e.yml -p ti3-e2e up -d --build postgres auth notes social
  python scripts/test-account-deletion-e2e.py
"""
import json
import subprocess
import time
import urllib.error
import urllib.request
import uuid

BASES = {"auth": "http://127.0.0.1:18085", "notes": "http://127.0.0.1:18082",
         "social": "http://127.0.0.1:18083"}
PASSWORD = "Sprint3-Test-Pass1!"
checks = 0


def request(service, method, path, expected=200, token=None, body=None):
    global checks
    hdr = {}
    if token:
        hdr["Authorization"] = "Bearer " + token
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        hdr["Content-Type"] = "application/json"
    req = urllib.request.Request(BASES[service] + path, data=data, headers=hdr, method=method)
    try:
        response = urllib.request.urlopen(req, timeout=150)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        status = response.status
    allowed = expected if isinstance(expected, tuple) else (expected,)
    assert status in allowed, f"{method} {path}: {status} not in {allowed}: {raw[:500]!r}"
    checks += 1
    parsed = json.loads(raw) if raw and raw[:1] in (b"{", b"[") else raw
    return status, parsed


def sql(query):
    return subprocess.check_output(["docker", "exec", "ti3-e2e-postgres", "psql", "-U", "postgres",
                                    "-d", "postgres", "-At", "-c", query], text=True).strip()


def register(label):
    email = f"delete-e2e-{uuid.uuid4().hex}@test.invalid"
    body = {"email": email, "password": PASSWORD, "display_name": label}
    _, user = request("auth", "POST", "/auth/register", 201, body=body)
    _, login = request("auth", "POST", "/auth/login", body={"email": email, "password": PASSWORD})
    return {"id": user["id"], "email": email, "token": login["access_token"],
            "refresh": login["refresh_token"]}


def main():
    for service in ("postgres", "notes", "auth", "social"):
        info = json.loads(subprocess.check_output(["docker", "inspect", f"ti3-e2e-{service}"]))[0]
        assert info["Config"]["Labels"]["com.docker.compose.project"] == "ti3-e2e"
    for service in BASES:
        assert request(service, "GET", "/health")[1]["status"] == "UP"

    owner = register("Borrado E2E autor")
    member = register("Borrado E2E miembro")
    a, b = owner["token"], member["token"]

    # Estado previo: grupo compartido (A admin, B miembro), grupo solo de A y
    # una nota de A compartida al grupo común.
    _, g = request("social", "POST", "/groups", 201, a, {"name": "Borrado E2E compartido"})
    shared_group = g["group_id"]
    _, invite = request("social", "POST", f"/groups/{shared_group}/invite/regenerate", 200, a)
    request("social", "POST", f"/groups/{shared_group}/join", 201, b,
            {"invite_token": invite["new_invite_token"]})
    _, g = request("social", "POST", "/groups", 201, a, {"name": "Borrado E2E solo A"})
    solo_group = g["group_id"]
    _, n = request("notes", "POST", "/notes", 201, a,
                   {"title": "Nota de A", "visibility": "private", "content": "# A\n"})
    note = n["note_id"]
    request("notes", "POST", f"/notes/{note}/share", 201, a,
            {"group_id": shared_group, "access_mode": "link"})
    _, gn = request("notes", "GET", f"/groups/{shared_group}/notes", 200, b)
    assert note in {x["id"] for x in gn["notes"]}
    request("notes", "GET", f"/notes/{note}", 200, b)
    print("PASS estado previo: 2 grupos, nota compartida visible para B", flush=True)

    # Antes de borrar.
    _, st = request("auth", "GET", "/auth/account-deletion", 404, a)
    assert st == {"status": "not_started"}
    request("auth", "DELETE", "/auth/account", 401)
    request("auth", "GET", "/auth/account-deletion", 401)

    # Borrado (mismo cuerpo vacío y JWT que envía el frontend).
    status, body = request("auth", "DELETE", "/auth/account", (200, 202), a)
    polls = 0
    while body.get("status") == "pending":
        assert status in (200, 202)
        assert polls < 20, "el borrado sigue pendiente tras ~60 s"
        time.sleep(3)
        polls += 1
        status, body = request("auth", "GET", "/auth/account-deletion", 200, a)
    assert body["status"] == "complete", body
    print(f"PASS DELETE /auth/account -> complete (consultas de estado: {polls})", flush=True)

    # Idempotencia y bloqueo del JWT anterior.
    _, again = request("auth", "DELETE", "/auth/account", 200, a)
    assert again == {"status": "complete"}
    _, st = request("auth", "GET", "/auth/account-deletion", 200, a)
    assert st["status"] == "complete" and st.get("updated_at")
    _, locked = request("auth", "GET", "/auth/me", 423, a)
    assert locked["error"]["code"] == "account_deletion_in_progress"
    request("notes", "GET", "/notes/me", 423, a)
    request("social", "GET", "/groups/me", 423, a)
    request("auth", "POST", "/auth/refresh", 401, body={"refresh_token": owner["refresh"]})
    request("auth", "POST", "/auth/login", 401, body={"email": owner["email"], "password": PASSWORD})
    print("PASS repetición 200, JWT viejo 423 en Auth/Notes/Social, refresh y login 401", flush=True)

    # Social: B hereda la administración; el grupo solo de A desaparece.
    _, members = request("social", "GET", f"/groups/{shared_group}/members", 200, b)
    assert [m["user_id"] for m in members] == [member["id"]], members
    assert members[0]["role"] == "admin", members
    assert sql(f"SELECT count(*) FROM social.groups WHERE id='{solo_group}'") == "0"
    assert sql(f"SELECT count(*) FROM social.group_memberships WHERE user_id='{owner['id']}'") == "0"
    # Notes: la compartición de A se retiró; B ya no la ve.
    _, gn = request("notes", "GET", f"/groups/{shared_group}/notes", 200, b)
    assert note not in {x["id"] for x in gn["notes"]}
    request("notes", "GET", f"/notes/{note}", 404, b)
    assert sql(f"SELECT count(*) FROM notes.shared_notes WHERE note_id='{note}'") == "0"
    # Política vigente: la nota (metadata y Drive del autor) se conserva.
    assert sql(f"SELECT count(*) FROM notes.notes WHERE id='{note}'") == "1"
    # Identity: usuario y dependientes borrados; tombstone completo.
    oid = owner["id"]
    assert sql(f"SELECT count(*) FROM identity.users WHERE id='{oid}'") == "0"
    assert sql(f"SELECT count(*) FROM identity.refresh_tokens WHERE user_id='{oid}'") == "0"
    assert sql(f"SELECT count(*) FROM identity.profiles WHERE user_id='{oid}'") == "0"
    assert sql(f"SELECT status FROM identity.account_deletions WHERE user_id='{oid}'") == "complete"
    print("PASS Social (sucesión y grupo vacío borrado), Notes (shares) e Identity (cascada)", flush=True)

    # La otra cuenta no se ve afectada.
    request("auth", "GET", "/auth/me", 200, b)
    request("auth", "POST", "/auth/login", 200, body={"email": member["email"], "password": PASSWORD})
    _, st = request("auth", "GET", "/auth/account-deletion", 404, b)
    assert st == {"status": "not_started"}
    print("PASS cuenta B intacta", flush=True)

    pending_while_social_down()
    pending_while_notes_table_missing()
    print(f"PASS {checks} respuestas verificadas; PostgreSQL real, Drive/Stream mock")


def pending_while_social_down():
    """Social caído: 202 pending con la cuenta ya bloqueada; el worker termina."""
    user = register("Borrado E2E pendiente")
    token = user["token"]
    subprocess.check_call(["docker", "stop", "ti3-e2e-social"], stdout=subprocess.DEVNULL)
    try:
        _, body = request("auth", "DELETE", "/auth/account", 202, token)
        assert body == {"status": "pending"}, body
        _, st = request("auth", "GET", "/auth/account-deletion", 200, token)
        assert st["status"] == "pending"
        request("auth", "GET", "/auth/me", 423, token)
        request("auth", "POST", "/auth/refresh", 401, body={"refresh_token": user["refresh"]})
        request("auth", "POST", "/auth/login", 401, body={"email": user["email"], "password": PASSWORD})
        # Aún no se borró nada en Identity: solo hay tombstone.
        assert sql(f"SELECT count(*) FROM identity.users WHERE id='{user['id']}'") == "1"
        print("PASS Social caído: 202 pending, cuenta bloqueada sin borrar filas", flush=True)
    finally:
        subprocess.check_call(["docker", "start", "ti3-e2e-social"], stdout=subprocess.DEVNULL)
    # El worker de Identity (cada 15 s) reanuda sin JWT; se consulta como el front.
    deadline = time.time() + 90
    while True:
        status, st = request("auth", "GET", "/auth/account-deletion", 200, token)
        if st["status"] == "complete":
            break
        assert time.time() < deadline, "el worker no completó el borrado pendiente"
        time.sleep(3)
    assert sql(f"SELECT count(*) FROM identity.users WHERE id='{user['id']}'") == "0"
    _, again = request("auth", "DELETE", "/auth/account", 200, token)
    assert again == {"status": "complete"}
    print("PASS worker reanudó sin JWT: pending -> complete; DELETE repetido 200", flush=True)


def docker_logs(container, since):
    out = subprocess.run(["docker", "logs", "--since", since, container], capture_output=True,
                         text=True, encoding="utf-8", errors="replace")
    return out.stdout + out.stderr


def wait_health(service, timeout=60):
    deadline = time.time() + timeout
    while True:
        try:
            if request(service, "GET", "/health")[1]["status"] == "UP":
                return
        except Exception:  # noqa: BLE001 - el servicio aún no escucha
            pass
        assert time.time() < deadline, f"{service} no respondió /health"
        time.sleep(1)


def pending_while_notes_table_missing():
    """Regresión de la prueba manual en Docker local (2026-10-24).

    Una base anterior al PR #94 no tenía notes.account_deletion_users: Notes
    BeginAccountDeletion fallaba con Internal y la cuenta quedaba pending con
    notes_begin_incomplete. Se reproduce ocultando la tabla (solo en la base
    aislada ti3-e2e), se comprueba el diagnóstico y que, al restaurarla, el
    worker completa el mismo borrado sin tocar identity.account_deletions.
    """
    user = register("Borrado E2E sin tabla Notes")
    token, uid = user["token"], user["id"]
    hidden = "account_deletion_users_e2e_hidden"
    since = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 2))
    sql(f"ALTER TABLE notes.account_deletion_users RENAME TO {hidden}")
    try:
        _, body = request("auth", "DELETE", "/auth/account", 202, token)
        assert body == {"status": "pending"}, body
        _, st = request("auth", "GET", "/auth/account-deletion", 200, token)
        assert st["status"] == "pending"
        row = sql(f"SELECT status||'|'||coalesce(last_error,'') FROM identity.account_deletions WHERE user_id='{uid}'")
        assert row == "pending|notes_begin_incomplete", row
        assert sql(f"SELECT count(*) FROM identity.users WHERE id='{uid}'") == "1"
        logs = docker_logs("ti3-e2e-notes", since)
        assert f"borrado de cuenta [begin] user_id={uid}" in logs, logs[-2000:]
        assert 'relation "notes.account_deletion_users" does not exist' in logs, logs[-2000:]
        print("PASS sin tabla en Notes: 202 pending, notes_begin_incomplete y causa en el log", flush=True)

        # Con el esquema incompleto Notes se niega a arrancar y nombra la migración.
        subprocess.check_call(["docker", "restart", "ti3-e2e-notes"], stdout=subprocess.DEVNULL)
        deadline = time.time() + 30
        while json.loads(subprocess.check_output(["docker", "inspect", "ti3-e2e-notes"]))[0]["State"]["Running"]:
            assert time.time() < deadline, "Notes arrancó con el esquema incompleto"
            time.sleep(1)
        logs = docker_logs("ti3-e2e-notes", since)
        assert "003_account_deletion.sql" in logs, logs[-2000:]
        print("PASS Notes no arranca sin la migración 003 y lo explica en el log", flush=True)
    finally:
        sql(f"ALTER TABLE notes.{hidden} RENAME TO account_deletion_users")
        subprocess.check_call(["docker", "start", "ti3-e2e-notes"], stdout=subprocess.DEVNULL)
    wait_health("notes")
    deadline = time.time() + 90
    while True:
        _, st = request("auth", "GET", "/auth/account-deletion", 200, token)
        if st["status"] == "complete":
            break
        assert time.time() < deadline, "el worker no completó el borrado tras restaurar la tabla"
        time.sleep(3)
    assert sql(f"SELECT count(*) FROM identity.users WHERE id='{uid}'") == "0"
    assert sql(f"SELECT status FROM notes.account_deletion_users WHERE user_id='{uid}'") == "complete"
    _, again = request("auth", "DELETE", "/auth/account", 200, token)
    assert again == {"status": "complete"}
    print("PASS tabla restaurada: el worker completó el mismo borrado; DELETE repetido 200", flush=True)


if __name__ == "__main__":
    main()
