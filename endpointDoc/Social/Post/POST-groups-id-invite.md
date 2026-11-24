# `POST /groups/:id/invite`

Invita a un usuario seguido a un grupo. La invitación queda pendiente hasta que su destinatario la acepte o rechace.

## Req

- `POST /groups/:id/invite`, protegido con JWT.
- `:id` es el UUID del grupo.
- JSON: `{"invited_user_id":"uuid"}`.
- Solo puede invitar un administrador del grupo y el destinatario debe ser una persona que sigue.

## Res

- `201`: `{"invitation_id":"uuid","status":"pending","message":"Invitación enviada"}`.
- `400` si el body o UUID es inválido, si el usuario ya es miembro o está baneado.
- `403` si el actor no administra el grupo o no sigue al destinatario.
- `409` si ya existe una invitación pendiente a ese grupo para la persona.
- `401` si falta o no es válido el JWT; `500` ante errores de persistencia.

## Flujo

El handler toma el usuario emisor del JWT. El servicio verifica rol admin, relación de seguimiento, membresía y bloqueo antes de insertar una fila pendiente en `social.group_invitations`.

## Front

`FollowedScreen` muestra el botón Invitar junto a cada persona seguida, lista los grupos administrados por el usuario y envía el UUID de esa persona al grupo seleccionado mediante `SocialService.inviteToGroup`.
