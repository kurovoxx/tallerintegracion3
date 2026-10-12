# Protos del monorepo (tarea 3_5_1: toolchain; 3_5_2: contratos).

## Comandos (desde la raíz del repo)

- `make tools` — instala `buf` + `protoc-gen-go` + `protoc-gen-go-grpc` en `./bin/`
  (los plugins se compilan en docker, no se exige Go local).
- `make generate` — genera los stubs Go de cada módulo (`back/proto/common`, `back/proto/social`;
  infacadem genera lo suyo con `back/proto/infacadem/generate.sh`).
- `make clean-tools` — borra `./bin/`.

## Estructura: un módulo por equipo

Cada equipo es dueño de su módulo (`buf.yaml` + `buf.gen.yaml` propios) y su
generación:

- `back/proto/common/` → stubs en `back/proto/gen/go/common/` (vía `make generate`).
- `back/proto/social/` → stubs en `back/social/internal/gen/socialv1/` (vía `make generate`).
- `back/proto/infacadem/` → stubs en `back/infacadem/internal/gen/` (vía su `generate.sh`).

## Regla

**Nunca correr `buf` en `back/proto` raíz** (ni `lint`, ni `generate`, ni con
`--path`/`--exclude-path`): el descubrimiento cruzado de módulos anidados es
inestable en buf y falla de forma no-determinista. Siempre dentro del módulo
(`cd back/proto/<modulo>`), o vía `make generate` / el `generate.sh` del equipo.

## Convenciones

- Un proto por dominio: `back/proto/<servicio>/v<N>/<recurso>.proto`
  (ej: `back/proto/social/v1/groups.proto`).
- `package <servicio>.v<N>;` — debe coincidir con el directorio (`buf lint` lo exige).
- `option go_package = "github.com/kurovoxx/tallerintegracion3/back/proto/gen/go/<servicio>/v<N>;<paquete>";`
- El código generado **se commitea** (los servicios lo importan como cualquier paquete).
