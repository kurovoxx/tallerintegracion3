# Dependencias externas al alcance de Héctor

## 3_2_16 — Eliminación de cuenta (resuelta 2026-10-23)

El bloqueo se levantó con el PR #94 (`feat/account-deletion-grpc`, Agustín Vega,
fusionado en Development como `05d63a3`): `DELETE /auth/account` y
`GET /auth/account-deletion` en Identity, con gRPC hacia Notes
(`BeginAccountDeletion`, `AccountDeletionCleanup`) y Social
(`PrepareAccountDeletion`, `AccountDeletionCleanup`). El frontend ya está
conectado (ver [docHector](docHector.md#borrado-de-cuenta-3_2_16)).

Pendientes que no son del frontend:

1. Política de contenido: Notes conserva las notas y archivos Drive del autor
   borrado (filas `notes.notes` huérfanas de `identity.users`). Si el producto
   decide borrarlos, corresponde a Notes/Identity.
2. `endpointDoc/GRPC-social-AccountDeletionCleanup.md` (Social) quedó
   desactualizado: no describe `PrepareAccountDeletion`, el campo `status` ni que
   la salida de Stream ahora es durable y condiciona `COMPLETE`.
3. Sin endpoint de cancelación: una vez iniciado, el borrado es irreversible.
4. Un JWT emitido sigue sirviendo para consultar el estado hasta su expiración
   (900 s por defecto); pasado ese plazo el cliente ya no puede consultar y el
   worker termina solo.

## Social — Duplicados de notificación

back/social/db/schema.sql define meeting_notifications con PK id y FK meeting_id
sin UNIQUE(meeting_id,user_id). Dedupe frontend por ID no resuelve dos registros
con IDs distintos. Sin identidad semántica de evento, tampoco corresponde que
el cliente los descarte por heurística.

Propuesta para responsable Social: auditar duplicados por SELECT agrupado,
acordar una notificación por reunión, consolidar is_read conservando lecturas,
agregar unicidad e INSERT ON CONFLICT compatible. Probar dos productores
concurrentes/reintentos y contador. No descartar filas sin decisión del dueño
del servicio. No se migró el esquema compartido en este cierre.
