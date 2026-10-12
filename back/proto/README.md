# Protos del monorepo (tarea 3_5_1: toolchain; 3_5_2: contratos).

## Comandos (desde la raíz del repo)

- `make tools` — instala `buf` + `protoc-gen-go` + `protoc-gen-go-grpc` en `./bin/`
  (los plugins se compilan en docker, no se exige Go local).
- `make generate` — genera los stubs Go en `back/proto/gen/go/`.
- `make clean-tools` — borra `./bin/`.

## Convenciones

- Un proto por dominio: `back/proto/<servicio>/v<N>/<recurso>.proto`
  (ej: `back/proto/social/v1/groups.proto`).
- `package <servicio>.v<N>;` — debe coincidir con el directorio (`buf lint` lo exige).
- `option go_package = "github.com/kurovoxx/tallerintegracion3/back/proto/gen/go/<servicio>/v<N>;<paquete>";`
- El código generado **se commitea** (los servicios lo importan como cualquier paquete).
