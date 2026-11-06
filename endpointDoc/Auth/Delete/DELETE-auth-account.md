# DELETE /auth/account

Orquesta el borrado completo de la cuenta propia del usuario de forma distribuida entre microservicios (Auth, Notes y Social) mediante gRPC.

## 1. Req (Petición)
- **Método y Ruta:** `DELETE /auth/account` (Protegida con JWT; `back/auth/cmd/server/main.go:120`, middleware `RequireIdentity`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`.
- **Cuerpo:** Sin cuerpo (el ID del usuario se extrae estrictamente del JWT).

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"complete"}` (Notas, Social e Identity confirmaron; `identity.users` eliminado). Repetir devuelve 200 de forma idempotente.
- **`202 Accepted`:** `{"status":"pending"}` (Tombstone registrado; un worker asíncrono reintenta en segundo plano).
- **`401 Unauthorized`:** Token inválido o ausente.
- **`404 Not Found`:** `account_not_found`.
- **`503 Service Unavailable`:** Fallo transitorio de commit o lock distribuido.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/account_deletion_service.dart:140` (`deleteAccount()`).
  - Diálogo UI: `front/lib/features/auth/account_deletion_dialog.dart:85` (exige escribir "ELIMINAR" para confirmar).
  - Pantalla: `front/lib/features/academic/profile_screen.dart:485`.

## 4. Flujo Técnico (Paso a Paso)
1. **Handler (`account_deletion_handler.go`):** Llama a `AccountDeletionCoordinator.DeleteAccount`.
2. **Fase 1 (Begin):** Registra tombstone `pending` en `identity.account_deletions` y bloquea nuevas peticiones concurrentes del usuario.
3. **Fase 2 (Notes gRPC `notes:9082`):** Llama a `BeginAccountDeletion` para bloquear escrituras y preparar revocaciones.
4. **Fase 3 (Social gRPC `social:9083`):** Llama a `PrepareAccountDeletion` y luego a `AccountDeletionCleanup` (sucesión de admin en grupos, salida de miembros, borrado de canales Stream).
5. **Fase 4 (Notes gRPC `notes:9082`):** Ejecuta `AccountDeletionCleanup` (retira shares y limpia permisos en Drive).
6. **Fase 5 (CompleteAndDelete):** Marca `complete` y elimina la fila de `identity.users` en cascada (borra perfil, resets, refresh tokens y conexiones oauth).

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (DELETE CASCADE), `identity.account_deletions`.
- **Llamadas gRPC:** `NotesService` (:9082) y `SocialService` (:9083) con autenticación `x-internal-key`.

## 6. Tests Asociados
- **Contrato:** `back/auth/internal/handler/http/account_deletion_contract_test.go`.
- **Coordinador:** `back/auth/internal/service/account_deletion_test.go`.
- **E2E Cruzado:** `back/social/test/cross_service_e2e_test.go` (`TestCrossServiceAccountDeletionRetriesPartialDriveCleanup`).
