#!/usr/bin/env bash
# Genera el código Go del contrato gRPC de Academic (hermético y reproducible).
# - Toolchain fijado (sin protoc/buf del sistema): se instala vía `go install`
#   si falta.
# - Salida commiteada en back/infacadem/internal/gen/academicv1 (no editar a mano).
# - Idempotente: correr dos veces deja diff vacío.
set -euo pipefail

BUF_VERSION="v1.59.0"
PROTOC_GEN_GO_VERSION="v1.36.12"
PROTOC_GEN_GO_GRPC_VERSION="v1.5.1"

cd "$(dirname "$0")"

export PATH="$PATH:$(go env GOPATH)/bin"

need_install() { ! command -v "$1" >/dev/null 2>&1; }

if need_install buf; then
  go install "github.com/bufbuild/buf/cmd/buf@${BUF_VERSION}"
fi
if need_install protoc-gen-go; then
  go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
fi
if need_install protoc-gen-go-grpc; then
  go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
fi

buf lint
buf generate
echo "generado OK en ../../infacadem/internal/gen/academicv1"
