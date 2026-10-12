# Documentación de tareas — Martín

Doc vivo: una sección por tarea del sprint 2 (3_5_N). Los endpoints van por
separado en `endpointDoc/` (un archivo por endpoint).

## 3_5_2 — Protos comunes (errores, paginación)

### Objetivo

Mensajes `.proto` compartidos que todos los servicios gRPC van a reutilizar.

### Archivos

| Archivo | Contenido |
|---|---|
| `back/proto/common/v1/errors.proto` | `Error { code, message, field }` + `ErrorResponse` (`package common.v1`) |
| `back/proto/common/v1/pagination.proto` | `PageRequest { page_size, page_token }` + `PageResponse { next_page_token }` |
| `back/proto/gen/go/common/v1/*.pb.go` | Generado commiteado (solo `common`; ver notas) |

### Decisiones

- **Códigos como `string`** (no `enum`): reusa el catálogo `snake_case` actual
  (`invalid_body`, `todo_not_found`, ...) sin reescribir validaciones; el código
  gRPC nativo lleva solo la categoría gruesa. Normalizar el catálogo queda para
  3_5_13.
- **Cursor sin `total_size`**: barato siempre (`limit+1`, como ya hace Notes);
  mapeo directo `page_token`=cursor, `page_size`=limit,
  `next_page_token`=`next_cursor`. El total se puede sumar después sin romper.

### Verificación

- `buf lint` (desde `back/proto/common`) limpio; `make generate` estable 3/3.
- `go build` + `go vet` del generado: `ok`.
- **No entra**: migrar handlers HTTP ni protos de dominio (tareas siguientes).

### Colisión con Infacadem (causa raíz + resolución)

- `back/proto/infacadem/academic.proto` (commit `e473cc3`, backlog 3_1_6) usa
  `package academic.v1` en directorio `infacadem/` y tiene su propio
  `buf.yaml` + `generate.sh` (salida en `back/infacadem/internal/gen`).
- Síntoma: `buf generate`/`lint` corridos desde `back/proto` raíz fallaban de
  forma **no-determinista** (mismo comando: 1/3 verde) con errores
  `PACKAGE_DIRECTORY_MATCH` y rutas relativizadas a la raíz del repo.
- Causa probada: el descubrimiento cruzado de módulos anidados en buf v1.73
  (`--exclude-path` tampoco lo aísla: 0/3; sin el módulo anidado: 3/3 estable).
  El rename solo (`package infacadem.v1` sin mover el archivo) tampoco basta:
  la regla exige dir `infacadem/v1`, y moverlo rompería la salida de su
  `generate.sh` (`source_relative` agregaría `/v1`).
- Resolución (sin tocar lo suyo): **un módulo por equipo**. La config de 3_5_1
  se movió a `back/proto/common/` (`buf.yaml` con `except:
  PACKAGE_DIRECTORY_MATCH` documentado + `buf.gen.yaml` con salida
  `../gen/go/common`); `make generate` corre `buf` dentro de cada módulo.
  Regla: nunca correr buf en `back/proto` raíz.

## 3_5_1 — buf + protoc-gen-go + buf.gen.yaml + make generate

Rama: `feat/buf-+-protoc-gen-go-+-buf.gen.yaml-+-make-generate`

### Objetivo

Montar la toolchain para generar stubs Go desde `.proto`, sin invocar `protoc`
a mano. No crea ningún endpoint ni ningún proto: solo configuración + Makefile.

### Archivos creados

| Archivo | Contenido |
|---|---|
| `back/proto/common/buf.yaml` | Módulo buf v2, lint `STANDARD`, breaking `FILE` (base del check de 3_5_9). Vive en el módulo (no en la raíz: ver 3_5_2) |
| `back/proto/common/buf.gen.yaml` | Plugins `protoc-gen-go` + `protoc-gen-go-grpc`, salida `../gen/go/common` |
| `Makefile` (raíz) | Targets `tools`, `generate` (por módulo, desde su dir), `clean-tools` |
| `back/proto/README.md` | Uso y convenciones de paquetes |
| `.gitignore` | `+ /bin/` (binarios locales, no se commitean) |

### Versiones fijadas

- `buf v1.73.0` (tarball oficial Linux x86_64)
- `protoc-gen-go v1.36.12` (igual que el runtime `protobuf` de los go.mod)
- `protoc-gen-go-grpc v1.6.2`
- Imagen de compilación: `golang:1.26` (igual que `go 1.26.0` de los servicios)

### Comandos

- `make tools` — deja `buf` + plugins en `./bin/` (los plugins se compilan en
  docker, no exige Go local).
- `make generate` — corre `buf generate` desde `back/proto` con `./bin` en el PATH.
- `make clean-tools` — borra `./bin/`.

### Convenciones (detalle en `back/proto/README.md`)

- Protos en `back/proto/<servicio>/v<N>/<recurso>.proto`.
- `package <servicio>.v<N>;` (debe coincidir con el directorio, lo exige el lint).
- `option go_package = ".../back/proto/gen/go/<servicio>/v<N>;<paquete>"`.
- El código generado **se commitea**.

### Verificación

Con un proto temporal (`smoke/v1/smoke.proto`, borrado tras verificar):

- `buf lint` limpio (se corrigió `DEFAULT` → `STANDARD` por deprecación en buf v1.73).
- `buf generate` produjo `smoke.pb.go` + `smoke_grpc.pb.go`.
- `go build` + `go vet` del generado: `ok`.

### Notas

- `make generate` falla con "no .proto files" hasta que la 3_5_2 agregue el
  primer proto real — esperado, no es bug.
- Siguiente: 3_5_2 (protos comunes: errores, paginación).
