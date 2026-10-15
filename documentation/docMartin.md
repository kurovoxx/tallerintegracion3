# Documentación de tareas — Martín

Doc vivo: una sección por tarea (3_5_N), en orden de ejecución. Endpoints por
separado en `endpointDoc/`.

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
- `make generate` — corre `buf generate` dentro de cada módulo con `./bin` en el PATH.
- `make clean-tools` — borra `./bin/`.

### Convenciones (detalle en `back/proto/README.md`)

- Protos en `back/proto/<servicio>/v<N>/<recurso>.proto`.
- `package <servicio>.v<N>;` — debe coincidir con el directorio (`buf lint` lo exige).
- `option go_package = ".../back/proto/gen/go/<servicio>/v<N>;<paquete>"`.
- El código generado **se commitea**.

### Verificación

Con un proto temporal (`smoke/v1/smoke.proto`, borrado tras verificar):

- `buf lint` limpio (se corrigió `DEFAULT` → `STANDARD` por deprecación en buf v1.73).
- `buf generate` produjo `smoke.pb.go` + `smoke_grpc.pb.go`.
- `go build` + `go vet` del generado: `ok`.

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

## 3_5_3 — Interceptores (auth interno, logging)

### Decisión

Opción B: copia por servicio, sin dependencias entre microservicios.
Implementado en `social` (`back/social/internal/interceptor/`); notes/auth
copian el mismo archivo cuando lo necesiten. No se tocó código de otros equipos.

### Archivos

| Archivo | Contenido |
|---|---|
| `back/social/internal/interceptor/auth.go` | `NewInternalAuth(secret)`: exige `x-internal-key` en metadata (equivale a `X-Internal-Key` HTTP). Sin secreto configurado → `Internal` (fail closed); sin key o mala → `Unauthenticated`. Comparación en tiempo constante |
| `back/social/internal/interceptor/logging.go` | `NewLogging(logger)`: registra método, duración y código por RPC (equivale a `logOp`/`fail` HTTP). Logger inyectable para tests |
| `back/social/internal/interceptor/interceptor_test.go` | 5 tests con `bufconn` (gRPC en memoria, servicio falso con `emptypb`/`wrapperspb`, sin protos ni red) |
| `back/social/go.mod` | `+ google.golang.org/grpc v1.83.2` (igual que notes) |

### Uso (cuando haya servidor gRPC)

```go
s := grpc.NewServer(grpc.ChainUnaryInterceptor(
    interceptor.NewLogging(log.Default()), // afuera: audita también rechazos
    interceptor.NewInternalAuth(cfg.InternalKey),
))
```

### Verificación

- `TestInternalAuth_AcceptsCorrectKey` / `RejectsMissingOrWrongKey` /
  `MisconfiguredFailsClosed` / `TestLogging_EmitsMethodAndCode` (OK + NotFound) /
  `TestChain_LoggingAuditsAuthRejection`: 5/5 PASS.
- `go build ./...` + `go test ./...` (social, con Postgres local): `ok` en
  todos los paquetes, sin regresiones.
- **No entra**: JWT de usuario (3_5_6), interceptores stream (sin uso hoy),
  cableado en servidores (no hay servidores gRPC propios aún).

## 3_5_4 — Conexión Notes→Social: share + checks is_admin/member

### Alcance

Lado Social (endpoints internos + middleware + métodos de servicio). Lado Notes
sin cambios: su `SocialHTTPAdapter` ya esperaba estos contratos y ahora
resuelven de verdad (antes 500 fail-closed contra Social real).

### Endpoints (tras `RequireInternalKey`, sin JWT)

| Ruta | Responde | Usa Notes para |
|---|---|---|
| `GET /internal/groups/:id/members` | `200 [{user_id, role}]` / `404` grupo | `IsMember`/`IsAdmin` al compartir (`note_service.go:1921,2312`) y `IsGroupAdmin` de `unshare-all` (desbloquea 3_5_5 kick/ban e2e) |
| `GET /internal/groups/:id/member-emails` | `200 [emails]` / `404` / `500` Identity caído | `ListMemberEmails` de shares `restricted` (`note_service.go:1975`) |

Fail-closed en emails: `500` si Identity falla (Notes aborta el share con
`ErrInternalDatabase`, no calcula permisos incompletos).

### Archivos

| Archivo | Contenido |
|---|---|
| `back/social/internal/middleware/internal_middleware.go` | `RequireInternalKey` (copia de auth, opción B): 401 sin/mala key, 503 sin secreto |
| `back/social/internal/handler/http/internal_handler.go` | `InternalHandler{Members, MemberEmails}` (arrays siempre, nunca null) |
| `back/social/internal/service/group_service.go` | `+ ListMembersInternal` (sin exigir membresía) y `+ ListMemberEmailsInternal` (fail-closed sin directorio) |
| `back/social/internal/service/user_directory.go` | `+ LookupEmailsStrict` en la interfaz + impl HTTP (normaliza minúsculas/dedupe/orden); `LookupUsers` best-effort intacto |
| `back/social/cmd/server/main.go` | Grupo `/internal` + 2 rutas |
| Tests | `internal_handler_test.go` (forma, 401/404/400/503, 500 fail-closed), `internal_members_test.go` (strict + internos), `internal_middleware_test.go` |

### Verificación

- `go test ./...` (social): `ok` todo, sin regresiones.
- **No entra**: `GET /internal/users/:id/followers-count` (sin lógica de
  follows en social; el share lo tolera best-effort con snapshot 0), cambios en
  Notes, migrar a gRPC (futuro; el contrato HTTP ya es el que el adapter pide).

## 3_5_5 — Conexión Social→Notes: unshare-all en leave/kick/ban

### Estado previo

`leave` ya llamaba a `POST /notes/unshare-all` (best-effort, reenviando el JWT
propio). `kick`/`ban` no llamaban: el expulsado no inicia la acción y no hay
JWT suyo que reenviar.

### Implementado (lado Social, sin tocar Notes)

- `GroupService.KickMember`/`BanMember` aceptan `cleanup_shared_notes` + `token`
  (misma firma que `LeaveGroup`): tras expulsar/banear con éxito disparan
  `go s.unshareNotes(target, group, actorToken)` con el **expulsado** como
  `user_id` y el **JWT del admin actor** (Notes lo autoriza vía `IsGroupAdmin`).
- Handlers `kick`/`ban` aceptan body opcional `{cleanup_shared_notes}` (igual
  que leave, default `false`) y extraen el token crudo del header.
- Llamadas existentes actualizadas a la firma extendida (12 call sites en tests).

### Archivos

| Archivo | Contenido |
|---|---|
| `back/social/internal/service/group_service.go:377-404` | Hook cleanup en kick/ban |
| `back/social/internal/handler/http/group_handler.go:200-226` | Flag + token en kick/ban |
| `back/social/internal/service/group_service_test.go` | `TestKickBanCleanSharedNotes` (kick/ban llaman con expulsado + Bearer admin, sin flag no llaman, kick fallido no llama) |

### Verificación

- Subtests 4/4 PASS; `go test ./...` (social, con Postgres): `ok` todo.

### Dependencia resuelta por 3_5_4

El llamado con JWT de admin necesitaba que Notes verifique `IsGroupAdmin` →
`GET /internal/groups/{id}/members`, que ya existe (ver 3_5_4 arriba). Kick/ban
e2e activos. `leave` funcionaba desde antes (JWT propio, sin check de admin).

## 3_5_6 — Conexión servicios→Identity: VerifyToken/GetUser en middlewares

### Alcance

Lado Identity (endpoints, con tests) + cliente en social (probado contra falso,
sin cableado productivo aún: los consumidores llegan en 3_5_4/3_5_5). Sin
cambios de comportamiento en prod. GetUser batch (`lookup`) ya existía y se
reusa; VerifyToken no existía (los servicios solo validaban firma local).

### Archivos

| Archivo | Contenido |
|---|---|
| `back/auth/.../handler/http/internal_auth_handler.go` | `POST /internal/auth/verify`: `{token}` → `200 {valid, user_id}` / `401 invalid_token\|token_expired` / `400` sin token. Reusa `JWTService.ValidarAccessToken` (sin DB) |
| `back/auth/.../handler/http/internal_users_handler.go` | `+ GetByID`: `GET /internal/users/:id` → `200 PublicUser` / `404 user_not_found` (reusa `UsersService.Lookup`) |
| `back/auth/internal/utils/response.go` | `+ ErrUserNotFound` (mensaje + map a 404). `400` usa el literal `bad_request` como los otros handlers internos |
| `back/auth/cmd/server/main.go:84,106-107` | Wire + registro de ambas rutas en el grupo `/internal` (tras `RequireInternalKey`) |
| `back/auth/.../handler/http/internal_auth_test.go` | Verify OK con JWT real, rechazos (malo/expirado/vacío), GetByID 200/404, sin-key 401 |
| `back/social/internal/identity/client.go` | `Client{VerifyToken, GetUser}` contra Identity con `X-Internal-Key` (copia local, opción B). Fail-closed: red/status inesperado = error, nunca éxito silencioso |
| `back/social/internal/identity/client_test.go` | Cliente contra Identity falso (200/401/404, key mala, sin baseURL) |
| `endpointDoc/POST-internal-auth-verify.md`, `GET-internal-users-id.md` | Docs de los 2 endpoints |

### Verificación

- Auth `go test ./...`: `ok` todo. Social `go test ./...` (con Postgres local): `ok` todo.
- **No entra**: cablear Verify en middlewares productivos (latencia por request;
  se usa donde importe: 3_5_4/3_5_5), interceptores gRPC sobre esto (3_5_3 ya
  cubre internal-key), cambios en Notes.

## 3_5_7 — Borrado de cuenta end-to-end Identity→Social→Notes

### Estado previo

Social tenía sucesión (`HandleAccountDeletion`) y el dueño de Social ya la
exponía por gRPC (`AccountDeletionCleanup`), pero sin limpieza en Notes: el
request solo traía `user_id` y sin JWT no hay cómo autorizar `unshare-all`.

### Implementado

- Proto (aditivo, compatible): `AccountDeletionCleanupRequest` suma
  `user_token` (JWT vigente que Identity reenvía ANTES de borrar su fila).
  Regen con `make generate` (diff mínimo: solo `social.pb.go`).
- `GroupService.HandleAccountDeletion` acepta `token`: tras la sucesión pide a
  Notes `unshare-all` en CADA grupo afectado (promovido o eliminado,
  best-effort, caller == user). Sin token solo hay sucesión.
- Handler gRPC reenvía `req.GetUserToken()`. Llamadas existentes actualizadas.

### Cadena completa (contrato para el orquestador de Identity)

1. Identity valida la identidad y toma el JWT vigente del usuario.
2. Identity llama `Social.AccountDeletionCleanup{user_id, user_token}` (gRPC
   `:9083`, auth interna 3_5_3).
3. Social hace sucesión por grupo y llama `POST /notes/unshare-all`
   `{user_id, group_id}` con el JWT (HTTP, 3_5_5).
4. Identity recién ahí borra su fila y revoca tokens.

### Archivos

| Archivo | Contenido |
|---|---|
| `back/proto/social/v1/social.proto` | `+ user_token = 2` (+ regen) |
| `back/social/internal/service/group_service.go` | Hook unshare por grupo en `HandleAccountDeletion` |
| `back/social/internal/handler/grpc/social_server.go` | Passthrough del token |
| `back/social/internal/service/group_service_test.go` | `TestAccountDeletionCleansNotesInAllGroups` (limpia ambos grupos con token; sin token solo sucesión) |

### Verificación

- Subtests 2/2 PASS; `go test ./...` (social): `ok` todo.
- **No entra**: el orquestador en Identity (tarea de su dueño; el contrato lo
  espera), migrar `unshare-all` a gRPC (tarea de Notes; el llamado sigue HTTP).

## 3_5_8 — Discovery: env/puertos gRPC, compose + manifests k8s

### Mapa fijado (ver `back/proto/README.md`)

HTTP `808x` ↔ gRPC `908x`: auth 9080 (reservado), infacadem 9081 (escuchando),
notes 9082 (reservado), social 9083 (escuchando). DNS internos = nombre del
Service. Server: env `GRPC_PORT` (social acepta `SOCIAL_GRPC_PORT`); cliente:
`<SVC>_GRPC_ADDR` (ej: `SOCIAL_GRPC_ADDR=social:9083`). Rango 9080-9089.

### Cambios

| Archivo | Contenido |
|---|---|
| `docker-compose.yml` | `GRPC_PORT` en auth/notes/social/infacadem + publish `908x:908x` host (infacadem ya tenía el env; social ya escuchaba) |
| `k8s/manifests.yaml` | `GRPC_PORT` en deployments auth/notes (reservado); infacadem `containerPort: 9081` + puerto `grpc` en Service `academic` (social ya lo tenía) |
| `back/proto/README.md` | Tabla del mapa + convención de envs |

Sin Services/containers gRPC para auth/notes hasta que existan sus servidores
(tareas de sus dueños); el mapa les reserva los puertos.

### Verificación

- `docker compose config`: válido, publica 9080-9083.
- k8s parsea (10 docs YAML).
- **No entra**: servidores gRPC de auth/notes, Ingress gRPC (solo HTTP hoy),
  Telepresence/vcluster.

## 3_5_14 — Notificación de reunión en app

### Alcance (jefe: front hace la cajita + ventana "próximas reuniones")

Backend: exponer lo que faltaba para esas dos vistas. El "enviar" ya existía
(fan-out a `meeting_notifications` + anuncio Stream); faltaba **leer**.

### Endpoints (JWT, sin cambios en contratos existentes)

| Ruta | Responde | Usa el front para |
|---|---|---|
| `GET /notifications/me` | `200 {data: [{id, meeting_id, group_id, title, scheduled_at, read, read_at, created_at}]}` (no leídas primero) | cajita de notificaciones |
| `POST /notifications/:id/read` | `200` / `404` ajena-inexistente-ya-leída | marcar leída |
| `GET /groups/:id/meetings` | `200 {data}` (`scheduled_at >= now`, cronológico, solo miembros) | ventana próximas reuniones |

### Archivos

| Archivo | Contenido |
|---|---|
| `back/social/internal/model/views.go` | `+ MeetingNotificationItem` |
| `back/social/internal/repository/meeting_repository.go` | `+ ListNotificationsByUser` (JOIN + no-leídas primero, SQL directo) y `+ MarkNotificationRead` (solo propia y no-leída) |
| `back/social/internal/service/notification_service.go` | `NotificationService` (interfaz angosta, sin romper `MeetingRepo`) |
| `back/social/internal/handler/http/notification_handler.go` | `ListMine` + `MarkRead` |
| `back/social/internal/handler/http/meeting_handler.go` | `+ ListMeetings` (reusa `ListUpcomingMeetings` existente) |
| `back/social/cmd/server/main.go` | Wire + 3 rutas |

### Verificación

- Unit (fakes): lista/forma/orden, read idempotente→404, solo-futura, 401/403/404.
- Integración PG: miembro ve 1, creador 0, `read_at` persiste, releída/ajena → 404.
- `go test ./...` (social): `ok` todo. Sin migración (tabla ya tenía `read_at`).
- Regla vigente (4.8): el fan-out excluye al creador; la cajita muestra las reuniones de los *demás*.

### Front (pedido del jefe: cajita + ventana, también hecho)

| Archivo | Contenido |
|---|---|
| `front/lib/core/models/social_models.dart` | `+ MeetingNotification` (fromJson tolerante) |
| `front/lib/core/services/social_service.dart` | `+ listMeetings`, `+ myNotifications`, `+ markNotificationRead` (mismo patrón `AuthedHttp`) |
| `front/lib/features/groups/group_detail_screen.dart` | (revertido: la pestaña `PRÓXIMAS` dedicada se eliminó; con el popup basta. Las próximas se ven en AGENDAR vía workspace) |
| `front/lib/features/groups/notifications_bell.dart` | `NotificationsBell` (badge no-leídas en header de grupos) + `NotificationsDialog` (lista, tap marca leída y recarga) |
| `front/lib/features/groups/groups_screen.dart` | Campana en header compacto y ancho |

- `flutter analyze` en los archivos tocados: sin issues nuevos (los restantes son preexistentes).
