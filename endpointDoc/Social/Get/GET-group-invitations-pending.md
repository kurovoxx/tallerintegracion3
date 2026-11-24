# `GET /group-invitations/pending`

Devuelve las invitaciones pendientes dirigidas al usuario autenticado. La app las muestra en la bandeja de notificaciones, con acciones para aceptar o rechazar.

## Req

- `GET /group-invitations/pending`, protegido con JWT.
- Sin body ni query. El destinatario se obtiene del JWT.

## Res

- `200`: `{"data":[{"id":"uuid","group_id":"uuid","group_name":"Grupo","invited_by_user_id":"uuid","invited_by_name":"Nombre","status":"pending","created_at":"RFC3339"}]}`.
- `invited_by_name` puede omitirse si el invitador no tiene nombre disponible. La lista se ordena de más reciente a más antigua.
- `401` si falta o no es válido el JWT; `500` si falla la consulta.

## Flujo

`GroupInvitationHandler.ListPending` → `GroupInvitationService.ListPendingInvitations` → `GroupInvitationRepository.ListPendingInvitations`. La consulta filtra por el usuario autenticado y estado `pending`.

## Front

`SocialService.listPendingInvitations` obtiene la lista. `GroupInvitationNotifications` muestra cada invitación en `NotificationInbox` y en la campana de grupos, junto con botones Aceptar y Rechazar.
