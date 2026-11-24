# `POST /group-invitations/:id/reject`

Rechaza una invitación pendiente dirigida al usuario autenticado.

## Req

- `POST /group-invitations/:id/reject`, protegido con JWT.
- `:id` es el UUID de la invitación. Sin body.
- El destinatario se obtiene del JWT.

## Res

- `200`: `{"status":"rejected","message":"Invitación rechazada"}`.
- `404` si la invitación no existe, pertenece a otro usuario o ya fue respondida.
- `400` si el UUID es inválido; `401` si falta o no es válido el JWT; `500` ante errores de persistencia.

## Front

`GroupInvitationNotifications` llama `SocialService.rejectInvitation` desde el botón Rechazar y quita la invitación de la bandeja al completar la operación.
