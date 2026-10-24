# Cierre Sprint 3 — Héctor Lepio (Notes)

Fecha de cierre: 2026-10-23 (actualizado la misma noche con 3_2_16). Rama de trabajo: `integration/notes-front-dev` (local, sin push).
Este informe solo contiene resultados ejecutados sobre el código final, salvo donde se indica. La §0 resume la última sesión; las §1–§5 describen el cierre anterior del mismo día y siguen vigentes.

## 0. Cierre definitivo: borrado de cuenta (3_2_16) e integración con Development

### Development integrado
- `git fetch`: `origin/Development` avanzó de `dc7667d` a `05d63a3` (PR #94 `feat/account-deletion-grpc`, autor `sonickiller39`): `b1a148c` orquestación Identity → Notes → Social, `c3f95c2` limpieza durable de Stream, `8ec43d7` aislamiento por sesión de perfil/calendario, `7a47a39` CI de protos contra Development.
- Respaldo previo: rama `backup/notes-front-dev-pre-account-deletion-merge-20261023` y parche del árbol en `.git/sprint3-evidence/worktree-pre-account-deletion-merge.patch`. Stashes intactos.
- Merge normal `20cf0ef` (sin rebase, reset ni push). Conflictos solo en pruebas:
  - `front/test/profile_drive_session_test.dart`: Development movió dos pruebas a `profile_drive_late_session_test.dart` y `profile_late_response_session_test.dart`; se conservó esa división (sin duplicados) con `servicePath` de la rama.
  - `front/test/schedule_meeting_screen_test.dart`: se conservaron ambos imports.
- `back/notes/internal/service/note_service.go` y `front/lib/features/academic/profile_screen.dart` se fusionaron sin conflicto; `go build`/`go vet` de auth, notes y social limpios.
- Fallo preexistente de Development detectado: `front/test/auth_service_test.dart` comparaba `url.path` con `/auth/login` y solo pasaba con `--dart-define` local. Corregido con `servicePath` en `aeb29b0` (no se desactivó).

### Contrato encontrado (verificado en el router y handlers)
| | `DELETE /auth/account` | `GET /auth/account-deletion` |
| --- | --- | --- |
| Ubicación | `back/auth/cmd/server/main.go:120`, `account_deletion_handler.go` | `main.go:121` |
| Auth | JWT Bearer con `RequireIdentity` (sirve aunque la cuenta ya esté bloqueada) | igual |
| Petición | sin body; sin contraseña; el usuario sale del JWT | — |
| Éxito | `200 {"status":"complete"}` (idempotente) / `202 {"status":"pending"}` | `200 {"status":"pending" o "complete","updated_at"}` |
| Errores | `404 {"error":"account_not_found"}`, `503 {"error":"account_deletion_unavailable"}`, `401` | `404 {"status":"not_started"}`, `503`, `401` |
| No usa | `403`, `409` | — |

Flujo síncrono con continuación asíncrona: Identity escribe un tombstone durable, llama por gRPC a Notes `BeginAccountDeletion`, Social `PrepareAccountDeletion` y `AccountDeletionCleanup` (sale de grupos con sucesión, borra el grupo si era el último miembro, baneos, Stream y `UnshareAll` en Notes), Notes `AccountDeletionCleanup` (shares y ACL de Drive) y al final borra `identity.users` (cascada de perfil, refresh tokens, resets y OAuth). Si una etapa no confirma devuelve `202` y un worker reintenta cada 15 s sin JWT. Desde el tombstone: rutas `RequireAuth` → `423 account_deletion_in_progress`, refresh y login → `401`. Notes **conserva** las notas y archivos Drive del autor. Detalle: `endpointDoc/DELETE-auth-account.md`, `endpointDoc/GET-auth-account-deletion.md`, `documentation/docHector.md#borrado-de-cuenta-3_2_16`.

### Frontend implementado (`817f64d`)
- `front/lib/core/services/account_deletion_service.dart`: cliente del contrato; single-flight por cuenta; timeout 45 s; respuestas ambiguas (red, timeout, `503`, `403/409/5xx`, cuerpo inesperado) → `GET` de estado antes de decidir; `finalizeLocal` (sesión de esa cuenta, purga SQLite por dueño en una transacción, correo recordado propio).
- `front/lib/features/auth/account_deletion_dialog.dart`: confirmación escrita `ELIMINAR`, fases confirmación / procesando / pendiente (sondeo 4 s × 15 y «Consultar estado») / no comenzó (reintentar) / sin confirmar (solo consultar) / sesión expirada; atrás y toque fuera bloqueados.
- `front/lib/features/academic/profile_screen.dart`: tarjeta «Cuenta» con badge «Zona de peligro» al final de Mi Perfil (no existe pantalla de ajustes); navegación al login con mensaje distinto para eliminada, inexistente y «sigue en curso».
- Componentes compartidos: `NeobrutalistDialog(actions:)`, `NeobrutalistBadge` con texto flexible; encabezado de `_ProfileCard` con `trailing` flexible (el badge de Drive desbordaba 14 px a 360 px / 200 %, defecto previo). `SessionManager.rememberedEmailKey` compartido con `login_screen`.
- Skill neobrutalista aplicada (`.skills/flutter_neobrutalism_ui.md`; la copia `.agents/skills/…/SKILL.md` solo añade frontmatter): tokens `AppColors`/`AppDimens`/`AppShadows`, `danger` con borde de 3 px, sin colores ni sombras sueltas (verificado con los `grep` del checklist), sin animación indefinida, semántica y área táctil ≥ 44 dp.

### Pruebas de esta sesión
| Suite | Resultado |
| --- | --- |
| `flutter test` sin defines | **384/384** (335 tras el merge + 49 nuevas) |
| `flutter test` con `API_BASE_URL`, `NOTES_BASE_URL`, `SOCIAL_BASE_URL`, `ACADEMIC_BASE_URL` locales | **384/384** |
| `account_deletion_service_test` + `account_deletion_flow_test` | 49/49, tres ejecuciones seguidas |
| `flutter analyze` | 0 errores; 28 observaciones, todas preexistentes (29 antes del merge) |
| Go Auth unitarias (filtro de CI) | todos los paquetes OK, incluye `account_deletion_contract_test.go` (`31c0fe5`) |
| Go con PostgreSQL aislado (bases `auth_test`, `notes_test`, `social_test` en `ti3-e2e-postgres`) | Auth 315 PASS / 0 FAIL / 1 SKIP preexistente (`TestIdentityServerPG`, usa otra variable); Notes 503/503; Social 373/373 con `-race -p 1` |
| `go test -tags=crossservice ./test` (Social; procesos reales) | 3/3, incluye `TestCrossServiceAccountDeletion*`. Log: `.git/sprint3-evidence/cross-service-e2e-20261023.log` |
| `scripts/test-account-deletion-e2e.py` (stack ti3-e2e reconstruido, volumen E2E recreado) | completo `200`, repetición `200`, JWT viejo `423` en Auth/Notes/Social, refresh/login `401`, sucesión de admin, grupo vacío borrado, share retirado, cascada Identity, cuenta ajena intacta; Social detenido → `202 pending` sin borrar filas → worker → `complete`. 44 respuestas. Log: `.git/sprint3-evidence/account-deletion-e2e-20261023.log` |
| `scripts/test-notes-e2e.py` | 21/21 rutas, 233 respuestas (`notes-e2e-post-account-deletion-20261023.log`) |

Solo se usaron cuentas sintéticas `@test.invalid` en el stack aislado `ti3-e2e` (puertos 1xxxx). No se tocó el stack `ti3-*` normal (arrancó solo al iniciar Docker Desktop) ni el clúster.

Fallos preexistentes no corregidos (fuera del alcance): `LoginScreen` desborda con texto al 200 % (`login_screen.dart:609`, `common_widgets.dart:75`); por eso las pruebas responsive del diálogo usan un host aislado y las de Mi Perfil cubren la tarjeta.

## 1. Estado inicial recuperado

- HEAD `153098d` (fin de la sesión anterior de Claude) + 74 archivos sin commit de Codex.
- Respaldos inmutables: `backup/notes-front-dev-wip-20261022` (trabajo de DeepSeek previo a la integración) y `backup/notes-front-dev-codex-20261023` (árbol completo dejado por Codex, creado antes de cualquier cambio).
- Stashes de GitHub Desktop conservados sin tocar (`stash@{0}`: solo `pubspec.lock`; `stash@{1}`: vacío).
- Worktree `tallerintegracion3-loadtest` no modificado.
- Logs de Codex en `.git/sprint3-evidence/`: `durable-restart.log` (11/11) pasaba solo al correr un archivo; `durable-after.log` mostraba un fallo real en `all_notes_screen_test` (aviso de adjunto fallido perdido); `notes-analysis.log` era de una versión anterior que no compilaba (`needsSync` requerido). Las 6 pruebas visuales de notas compartidas fallaban.

## 2. Trabajo heredado

### DeepSeek (preservado en la sesión anterior)
Idempotencia de creación, limpieza de filas fantasma, tombstones, `AppDatabaseManager`, cierre controlado de SQLite, aislamiento por dueño y entorno E2E aislado (`docker-compose.e2e.yml`).

### Sesión anterior de Claude
Causa raíz de notas duplicadas (fila temporal + UUID coexistiendo por timeout de 8 s y `_replaceLocalId` abortado al desmontar), promoción atómica temp→UUID, timeout 45 s, single-flight de POST, borrado durante creación, paginación de `GET /notes/me`, escrituras SQLite tras desmontar, campana de notificaciones, regresiones y pruebas PostgreSQL de idempotencia.

### Codex (revisado; conservado salvo lo indicado en §3)
- Sincronización: POST en vuelo aislado por cuenta/revisión, listado antiguo que no pisa ediciones, cursores cíclicos, IDs temporales monotónicos.
- Adjuntos offline durables: esquema SQLite v5 (`needs_sync`, `pending_note_attachments`), recibo `uploaded_id`, migración, build_runner.
- Mocks de prueba independientes del prefijo `/api-*` (`test/support/service_uri.dart`); `api_config` y pinning de Pillán verificables.
- Documentación: 21 rutas HTTP + gRPC en `endpointDoc/`, `docHector.md`, `notes_api_matrix.md`, `sprint3_dependencias.md`; `scripts/test-notes-e2e.py`; pruebas Go de contrato del router y `ListMyNotes`.

## 3. Correcciones nuevas de esta sesión

| Defecto | Evidencia | Corrección |
| --- | --- | --- |
| Paginación de notas de grupo omitía comparticiones (cursor solo por `shared_at`, ignoraba `is_admin_note`) | PostgreSQL real: 2 de 6 alcanzables con el SQL anterior; `TestListGroupNotesPaginationAndOrdering` fallaba ~1/20 | Keyset total `(is_admin_note, shared_at, id)` en PG y memoria; `TestSharedListByGroupKeysetTiesAndAdmin` |
| Aviso "no se pudo adjuntar <archivo>" eliminado por Codex y aserción debilitada | `durable-after.log`; diff de `all_notes_screen_test` | Mensaje restaurado con nombres; aserción original restaurada |
| Offline pedía "volver a seleccionar" archivos que ya estaban guardados (provocaría adjuntos duplicados) | Lectura del pipeline | Mensaje según persistencia real |
| Bytes ilimitados en SQLite | Esquema sin límites | 10 MB por archivo, 100 MB pendientes por cuenta; si no caben se avisa |
| Mapa en memoria de adjuntos no se liberaba tras subir | Diff de Codex | Se libera tras registrar el recibo |
| Cabecera de notas compartidas: título de 13 px de ancho (una letra por línea) a 360 px y texto 200 % | Medición en prueba: 544 px de alto | Botón de ícono con tooltip en anchos angostos o texto ampliado |
| Overflows de 35–93 px (botones de modo, acciones de `NeobrutalistDialog`) y 10 px vertical en estados | Pruebas visuales | `Wrap` y scroll |
| Pruebas visuales con selector inexistente (`tooltip 'Cerrar'`) y toque sobre botón tapado | Excepción `No element` | Selector `CERRAR` y `ensureVisible` |
| Suite bloqueada tras integrar Development (3_4_14): `SessionManager._persistenceQueue` estático encadenaba escrituras a un futuro creado en la zona de reloj simulado de una prueba ya terminada; el `clear()` siguiente esperaba para siempre | Sonda con trazas: el cuerpo terminaba y `tearDown` no; también se colgaba en `origin/Development` puro (`group_members_tab_test`) | Con la cola ociosa, la nueva escritura parte de un futuro en la zona actual; con escrituras en curso se conserva el orden. Sin cambio de comportamiento en producción; `session_manager_test` y `session_refresh_test` (del dueño de 3_4_14) en verde |
| Mocks de refresh sin rotación de `refresh_token` en 3 pruebas de Notes, obsoletos tras el contrato de rotación obligatoria de 3_4_14 | `AuthedHttp` trata un refresh sin rotación como transitorio | Mocks actualizados para devolver un token rotado (contrato real) |
| `docHector.md`: PATCH descrito como caché de 10 min | `UpdateWithExpectedVersion` usa `notes.idempotency_keys` | Documentación y comentario del handler corregidos |
| Mojibake y espacios finales en archivos de Codex | `git diff --check` | Corregidos (`dart format` solo en archivos del alcance) |

## 4. Validación SQLite / adjuntos offline (escenarios 1–15)

Base SQLite en **archivo real**, reinicios cerrando y reabriendo la base, bytes nunca recuperados de memoria (`debugClearPendingAttachments`):

- `notes_sync_duplicates_regression_test.dart` — *diálogo offline con imagen*: crear offline desde el diálogo real (1–2), bytes en SQLite con dueño (5), reinicio (3–4), reconexión: un POST y **una** subida (6–7), PATCH fallido deja `needs_sync=true` y recibo (8), segundo reinicio (9), PATCH correcto sin re-subir (10), contenido original preservado (11), sin filas temporales ni pendientes huérfanos (15).
- *adjunto offline sobrevive reinicios y PATCH fallido no vuelve a subirlo* (Codex): fallo de subida → subida OK + PATCH fallido → PATCH OK; 2 subidas en total (1 fallida, 1 confirmada).
- `app_database_migration_test.dart`: migración v4→v5 sobre archivo real con nota temporal, intención y tombstone (13); borrado solo de los pendientes de la nota (12); re-asociación al promover sin huérfanos (15); aislamiento entre cuentas (14); límites de tamaño y cuota.
- Transacciones: nota + bytes en una transacción; promoción temp→UUID (re-asociación incluida) en una transacción; borrado de nota + pendientes en una transacción.

## 5. Validación visual

`group_notes_tab_test.dart`: 360×640, 768×1024 y 1440×900 × escala 1.0 y 2.0 — carga, vacío, lista, visor, diálogo de compartir y error, sin excepciones ni overflow (10/10). Capturas regeneradas con `--dart-define=SPRINT3_CAPTURE=true` en `.git/sprint3-evidence/shared-{360,768,1440}.png`.

## 6. Resultados de pruebas (código final)

### Flutter
Código integrado final (con `origin/Development` al 2026-10-23):

| Configuración | Resultado |
| --- | --- |
| `flutter test` (sin defines, bases de Pillán) | **325/325** |
| `flutter test` con `API_BASE_URL`, `NOTES_BASE_URL`, `SOCIAL_BASE_URL` locales | **325/325** |

Referencias anteriores, de otros estados del código (no comparables directamente): DeepSeek 242/32; sesión anterior de Claude 270/274 con defines; Codex ~216/290 sin defines. Antes del merge final, esta sesión obtuvo 308/308 en ambas configuraciones. Ninguna prueba se desactivó ni se marcó como `skip`.

`flutter analyze`: 0 errores; 29 observaciones, todas preexistentes en módulos ajenos (social_models, attendance, login, etc.). Línea base de la sesión anterior: 36.

### Go
- Notes con PostgreSQL real (`NOTES_TEST_DATABASE_URL` → contenedor aislado `ti3-e2e-postgres`), sobre el código integrado final: 8/8 paquetes OK, **494 casos PASS, 0 FAIL, 0 SKIP**. `go vet` limpio.
- `TestListGroupNotesPaginationAndOrdering`: 200/200 ejecuciones tras la corrección.
- Auth (unitarias, mismo filtro que CI) y Social tras integrar Development: todos los paquetes OK.

### E2E (stack `ti3-e2e` aislado: Auth JWT real, Notes, Social, PostgreSQL real, Drive mock)
- Ejecutado de nuevo tras integrar Development, con el volumen E2E recreado para aplicar el `init.sql` actualizado.
- `scripts/test-notes-e2e.py`: **21/21 rutas con éxito, 233 respuestas verificadas** (incluye nueva paginación de notas de grupo: 5 comparticiones en 3 páginas sin omisiones). Logs: `.git/sprint3-evidence/notes-e2e-final-merged-20261023.log` (final) y `notes-e2e-final-20261023.log` (antes del merge).
- Idempotencia de creación (reintento secuencial, 6 concurrentes, reintento tras 24 h, payload distinto → 409, sin duplicados en `GET /notes/me`): 9/9. Log: `.git/sprint3-evidence/notes-idem-e2e-merged-20261023.log`.
- Solo se crearon y borraron datos sintéticos del contenedor E2E. No se usaron bases reales ni Kubernetes.

## 7. Matriz de endpoints

Documentados en `endpointDoc/` y verificados en E2E: `GET /health`, `GET /health/drive`, `POST /notes`, `GET /notes/me`, `GET /notes/{id}`, `PATCH /notes/{id}`, `DELETE /notes/{id}`, `GET /notes/{id}/access`, `POST /notes/reconcile`, `POST /notes/upload`, `POST /notes/{id}/attachments`, `GET /notes/{id}/attachments/{attachmentId}/content`, `DELETE /notes/{id}/attachments/{attachmentId}`, `POST /notes/{id}/save`, `POST /notes/{id}/copy`, `POST /notes/{id}/like`, `DELETE /notes/{id}/like`, `POST /notes/{id}/share`, `DELETE /notes/shared/{sharedNoteId}`, `POST /notes/unshare-all` (deprecado, reemplazado por gRPC), `GET /groups/{id}/notes`. gRPC `notes.v1.NotesService` (10 RPC) en `endpointDoc/GRPC-notes-NotesService.md`. `TestHTTPRouterContract` falla si aparece una ruta sin contrato. Matriz de cobertura: `documentation/notes_api_matrix.md`.

## 8. Estado de tareas

| Tarea | Estado | Evidencia |
| --- | --- | --- |
| 3_2_1 Protos Notes | Completada y verificada | `back/proto/notes/v1/notes.proto`; código generado compila; CI `grpc-ci.yml` (buf) |
| 3_2_2 Servidor gRPC | Completada y verificada | `handler/grpc/server.go`; suite bufconn verde |
| 3_2_3 Permisos Drive restricted | Completada y verificada (con Drive mock) | `TestHandlerShareRestrictedGrantsDrive`, E2E share restricted; permisos Google reales no probados contra Google |
| 3_2_4 Migración HTTP→gRPC | Completada y verificada | Social usa `NotesService/UnshareAll`; HTTP marcado deprecado; E2E Social↔Notes |
| 3_2_5 Tests gRPC | Completada y verificada | `server_test.go`, `list_my_contract_test.go` |
| 3_2_6 E2E Notes–Social | Completada y verificada | `back/notes/test/notes_social_e2e_test.go` + `scripts/test-notes-e2e.py` |
| 3_2_7 Puertos gRPC Docker/K8s | Completada y verificada (sin desplegar) | 9082 en compose y `k8s/manifests.yaml`; YAML validados localmente |
| 3_2_8 Estilos notas compartidas | Completada y verificada | 10 pruebas visuales, capturas, correcciones de overflow |
| 3_2_9 Documentación endpoints y lógica | Completada y verificada | `endpointDoc/`, `docHector.md` contrastados con handlers |
| 3_2_10 endpointDoc Notes | Completada y verificada | 21 rutas + gRPC |
| 3_2_11 Integridad código/documentación | Completada y verificada | Afirmaciones erróneas corregidas (gRPC, PATCH); router contract test |
| 3_2_12 Unitarios, integración y E2E | Completada y verificada | §6 |
| 3_2_13 Flujos Notes | Completada y verificada | `docHector.md` (secciones de sincronización, SQLite, ACL) |
| 3_2_14 Ventana de miembros | Completada y verificada | `group_members_tab_test.dart` (incluye cambios de roles de Development) |
| 3_2_15 Ventana de notas compartidas | Completada y verificada | Paginación corregida y probada en PG/E2E; pruebas de la pestaña |
| 3_2_16 Borrado de cuenta (frontend) | **Completada y verificada** | `DELETE /auth/account` + `GET /auth/account-deletion` (PR #94) conectados desde Mi Perfil; confirmación escrita, pendiente/ambigüedad sin éxito falso, limpieza local por dueño; 49 pruebas Flutter, contrato Go y E2E real (§0) |
| 3_2_17 Estilos de botones | Completada y verificada | `iteracion_vistas_reales_test.dart`, `NeobrutalistDialog` responsive |
| 3_2_18 SQLite y notificaciones | Completada y verificada | §4; `notifications_bell_state_test.dart`; `app_database_manager_test.dart` |

## 9. Git

### Commits de la sesión final (sobre `dab7f2e`)
- `20cf0ef` Merge origin/Development into integration/notes-front-dev (borrado de cuenta 3_x, PR #94)
- `aeb29b0` test(front): auth_service_test independiente de la base configurada
- `817f64d` feat(front): eliminacion de cuenta desde Mi Perfil (3_2_16)
- `22ea614` test(front): contrato HTTP y flujo de eliminacion de cuenta
- `31c0fe5` test(auth): contrato HTTP del borrado de cuenta y E2E ti3-e2e
- commit de documentación (este informe, `docHector.md`, `sprint3_dependencias.md`, `endpointDoc/DELETE-auth-account.md`, `endpointDoc/GET-auth-account-deletion.md`)

`origin/Development` (`05d63a3`) es ancestro de la rama; sin push. Sin trailers de coautoría (regla `.agents/rules/no-coauthor-trailer-in-commits.md`). Respaldo nuevo: `backup/notes-front-dev-pre-account-deletion-merge-20261023`.

### Cierre anterior del mismo día

Commits locales de `integration/notes-front-dev` que no están en `origin/Development` (18 + este informe), en orden:

- `a930321` fix(notes): limpiar fila fantasma ante rechazo definitivo de Drive en Create
- `c3b1a4a` feat(front-notes): creacion idempotente, tombstones y AppDatabaseManager
- `1cb96c2` feat(front): vistas academicas responsive, estilos de grupos y notificaciones
- `199ebd0` chore(e2e): entorno E2E local aislado (PostgreSQL + mocks) y run-front windows-local
- `94082a2` Merge origin/Development into integration/notes-front-dev (Sprint 3: identity gRPC, HPA, CI gRPC, mocks)
- `7748185` fix(front-notes): eliminar la causa de las notas duplicadas y perdidas
- `2abebd3` fix(front): campana de notificaciones sin popups duplicados y estado consistente (3_2_18)
- `c23d1d6` test(front): cubrir AppDatabaseManager y alinear tests de grupos al restyle
- `ff17de5` test(notes): reintento de Create tras vencer la ventana de replay no duplica
- `c016a14` test(notes): claim de idempotencia vencido conserva resource_id en PostgreSQL
- `7a36a4a` fix(notes): paginacion keyset total de notas compartidas por grupo (3_2_15)
- `f2303cf` test(notes): contrato del router HTTP productivo y ListMyNotes gRPC
- `362008b` test(front): api_config y pinning de Pillan verificables sin relajar TLS
- `2caf1ad` feat(front-notes): sincronizacion robusta y adjuntos offline durables (SQLite v5)
- `2724866` fix(front): notas compartidas responsive en movil y texto al 200% (3_2_8)
- `276d288` docs(notes): endpointDoc de 21 rutas + gRPC, docHector y E2E de contrato (3_2_9/10/11/13)
- `d6f1c8d` Merge origin/Development into integration/notes-front-dev (3_4_14 sesion persistente, roles Social)
- `0f39e29` fix(front): cola de persistencia de sesion no queda colgada entre zonas

- Relación con Development: `origin/Development` (`dc7667d`) es ancestro de la rama; diferencia: 18 commits por delante, 0 por detrás (102 archivos).
- **No se fusionó en `Development` local**: la rama incluye cambios fuera del alcance de Notes (refactor responsive académico heredado del árbol de DeepSeek, `629ec90`→`1cb96c2`, y el endurecimiento de `SessionManager` de 3_4_14). Deben revisarse en el PR con sus responsables.
- `Development` local y las ramas Sprint 3 del usuario (`feature/notes-*`, `feature/front-shared-notes-fixes`), que no tenían commits propios, se adelantaron por fast-forward a `origin/Development`. Ramas de otros integrantes y del Sprint 1-2 sin cambios.
- Respaldos: `backup/notes-front-dev-wip-20261022`, `backup/notes-front-dev-codex-20261023`, `backup/notes-front-dev-pre-trailer-cleanup-20261023` (estado previo a quitar trailers). Stashes de GitHub Desktop intactos. Sin push.

- Excluidos a propósito (sin commit): registrantes de plugins Flutter (solo finales de línea) y `front/pubspec.lock` (downgrades del SDK local; impondría versiones al equipo).
- Regla del repositorio `.agents/rules/no-coauthor-trailer-in-commits.md` (llegó con el merge): los 17 commits locales previos que tenían trailer `Co-Authored-By` se recrearon sin él con `git commit-tree` (mismos árboles, autor y fechas; árbol final idéntico, verificado). Ningún commit publicado se reescribió.

## 10. Dependencias bloqueadas y pendientes

1. **3_2_16** (resuelto): queda como decisión de producto si se borran las notas y archivos Drive del autor (hoy se conservan); `endpointDoc/GRPC-social-AccountDeletionCleanup.md` (Social) está desactualizado respecto a `PrepareAccountDeletion`/`status`; no hay cancelación del borrado. Ver `documentation/sprint3_dependencias.md`.
2. **Social (otro responsable)**: `social.meeting_notifications` sin `UNIQUE(meeting_id, user_id)`; el frontend deduplica por id pero no puede resolver duplicados con ids distintos.
3. Permisos Drive restricted contra Google real no se probaron (solo mock), por diseño del entorno aislado.

## 11. Recomendaciones para la integración remota

1. Revisar el diff `origin/Development...integration/notes-front-dev` y abrir PR hacia `Development` (no hacer push forzado).
2. En el PR, ejecutar `flutter test` (sin defines) y `go test ./...` en `back/notes`; opcionalmente las pruebas PG con `NOTES_TEST_DATABASE_URL` sobre una base desechable.
3. Al desplegar, la app migrará SQLite v4→v5 automáticamente; no requiere acción del usuario.
4. Coordinar con Social la unicidad de notificaciones y con Notes/Identity la política de contenido de cuentas borradas.
5. En el PR, `scripts/test-account-deletion-e2e.py` sirve como verificación manual contra el stack `ti3-e2e`; no se ejecutó contra Pillán.
