# Dependencias externas al alcance de Héctor

## 3_2_16 — Eliminación de cuenta

Revisión de back/auth/cmd/server/main.go, handlers y back/proto/auth: no existe
DELETE /auth/me, DELETE /account ni RPC DeleteAccount. DELETE de conexión Drive
solo desconecta OAuth. Social AccountDeletionCleanup limpia membresías/shares;
no borra identity.users ni coordina Notes/Academic ni invalida todo JWT.

Integración frontend preparada como contrato pendiente, sin simular éxito:

1. Identity debe acordar ruta autenticada, confirmación/reautenticación, respuesta
   final o pendiente y semántica idempotente de reintentos.
2. Actor será sub autenticado, nunca user_id arbitrario del cliente.
3. Backend coordinará revocación refresh/OAuth, destino de notas/archivos y
   limpieza por servicio, con compensación. Acordar conservación de contenido
   compartido antes de cascadas.
4. Frontend confirmará explícitamente, mostrará avance/error recuperable y solo
   limpiará sesión/navegará tras confirmación backend. SQLite se limpia por owner.
5. Aceptación: cancelar, sesión expirada, timeout ambiguo + replay, limpieza
   parcial/final, navegación y aislamiento de otra cuenta.

No se creó ruta inventada ni se borraron usuarios reales.

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
