# Documentación de tareas — Martín

Doc vivo: una sección por tarea del sprint 2 (3_5_N). Los endpoints van por
separado en `endpointDoc/` (un archivo por endpoint).

## 3_5_1 — buf + protoc-gen-go + buf.gen.yaml + make generate

Rama: `feat/buf-+-protoc-gen-go-+-buf.gen.yaml-+-make-generate`

### Objetivo

Montar la toolchain para generar stubs Go desde `.proto`, sin invocar `protoc`
a mano. No crea ningún endpoint ni ningún proto: solo configuración + Makefile.

### Archivos creados

| Archivo | Contenido |
|---|---|
| `back/proto/buf.yaml` | Módulo buf v2 (`path: .`), lint `STANDARD`, breaking `FILE` (base del check de 3_5_9) |
| `back/proto/buf.gen.yaml` | Plugins `protoc-gen-go` + `protoc-gen-go-grpc`, salida única `gen/go` |
| `Makefile` (raíz) | Targets `tools`, `generate`, `clean-tools` |
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
