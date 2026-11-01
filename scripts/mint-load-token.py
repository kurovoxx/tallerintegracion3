#!/usr/bin/env python3
"""Acuña un JWT HS256 de carga y lo guarda en el Secret loadgen-token.

Lee JWT_SECRET del Secret ti3-secrets, firma un token con iss=apuntes-auth,
aud=apuntes-client y claim user_id, y crea/actualiza el Secret loadgen-token.
El token nunca se imprime ni se pasa por argumentos de línea de comandos.

Solo usa la librería estándar de Python 3.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import hmac
import json
import subprocess
import sys
import time

DEFAULT_NAMESPACE = "student-brojas"
DEFAULT_SOURCE_SECRET = "ti3-secrets"
DEFAULT_SOURCE_KEY = "JWT_SECRET"
DEFAULT_TARGET_SECRET = "loadgen-token"
DEFAULT_TARGET_KEY = "TOKEN"
DEFAULT_USER_ID = "24e120f9-d698-442c-999e-56b55737985a"
DEFAULT_EXPIRATION = "24h"
ISSUER = "apuntes-auth"
AUDIENCE = "apuntes-client"


def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def parse_duration(value: str) -> int:
    """Convierte 90, 90s, 30m, 24h o 7d a segundos."""
    text = str(value).strip().lower()
    if not text:
        raise ValueError("la expiración no puede estar vacía")
    multipliers = {"s": 1, "m": 60, "h": 3600, "d": 86400}
    unit = text[-1]
    if unit in multipliers:
        number = text[:-1]
        factor = multipliers[unit]
    else:
        number = text
        factor = 1
    if not number.isdigit():
        raise ValueError(f"expiración inválida: {value!r} (usa 30m, 24h o 7d)")
    seconds = int(number) * factor
    if seconds <= 0:
        raise ValueError("la expiración debe ser mayor que cero")
    return seconds


def mint_token(secret: bytes, user_id: str, expires_in: int, now: int | None = None) -> str:
    issued_at = int(time.time()) if now is None else int(now)
    header = {"alg": "HS256", "typ": "JWT"}
    payload = {
        "user_id": user_id,
        "iss": ISSUER,
        "aud": AUDIENCE,
        "iat": issued_at,
        "exp": issued_at + int(expires_in),
    }
    signing_input = ".".join(
        [
            b64url(json.dumps(header, separators=(",", ":")).encode("utf-8")),
            b64url(json.dumps(payload, separators=(",", ":")).encode("utf-8")),
        ]
    )
    signature = hmac.new(secret, signing_input.encode("ascii"), hashlib.sha256).digest()
    return f"{signing_input}.{b64url(signature)}"


def run_kubectl(args: list[str], stdin: str | None = None) -> str:
    try:
        result = subprocess.run(
            ["kubectl", *args],
            input=stdin,
            text=True,
            capture_output=True,
            check=False,
        )
    except FileNotFoundError as exc:
        raise RuntimeError("kubectl no está instalado o no está en PATH") from exc
    if result.returncode != 0:
        detail = (result.stderr or result.stdout or "").strip()
        raise RuntimeError(f"kubectl {' '.join(args)} falló: {detail}")
    return result.stdout


def read_jwt_secret(namespace: str, source_secret: str, source_key: str) -> bytes:
    encoded = run_kubectl(
        [
            "get",
            "secret",
            source_secret,
            "-n",
            namespace,
            "-o",
            f"jsonpath={{.data.{source_key}}}",
        ]
    ).strip()
    if not encoded:
        raise RuntimeError(
            f"el Secret {source_secret}/{namespace} no tiene la clave {source_key}"
        )
    try:
        return base64.b64decode(encoded, validate=True)
    except ValueError as exc:
        raise RuntimeError(f"{source_key} no está en base64 válido") from exc


def upsert_target_secret(namespace: str, name: str, key: str, token: str) -> None:
    manifest = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": name, "namespace": namespace},
        "type": "Opaque",
        "stringData": {key: token},
    }
    run_kubectl(["apply", "-f", "-"], stdin=json.dumps(manifest))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Acuña un JWT de carga y actualiza el Secret loadgen-token."
    )
    parser.add_argument("--namespace", default=DEFAULT_NAMESPACE)
    parser.add_argument("--source-secret", default=DEFAULT_SOURCE_SECRET)
    parser.add_argument("--source-key", default=DEFAULT_SOURCE_KEY)
    parser.add_argument("--target-secret", default=DEFAULT_TARGET_SECRET)
    parser.add_argument("--target-key", default=DEFAULT_TARGET_KEY)
    parser.add_argument("--user-id", default=DEFAULT_USER_ID)
    parser.add_argument(
        "--exp",
        default=DEFAULT_EXPIRATION,
        help="expiración: 90, 90s, 30m, 24h o 7d (default: 24h)",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    user_id = args.user_id.strip()
    if not user_id:
        print("error: --user-id no puede estar vacío", file=sys.stderr)
        return 2

    try:
        expires_in = parse_duration(args.exp)
        secret = read_jwt_secret(args.namespace, args.source_secret, args.source_key)
    except (ValueError, RuntimeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    token = mint_token(secret, user_id, expires_in)
    try:
        upsert_target_secret(args.namespace, args.target_secret, args.target_key, token)
    except RuntimeError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(
        f"OK: Secret {args.target_secret} actualizado en {args.namespace} "
        f"(user_id={user_id}, exp={args.exp})."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
