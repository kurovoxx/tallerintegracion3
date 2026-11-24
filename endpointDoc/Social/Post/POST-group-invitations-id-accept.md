# `POST /group-invitations/:id/accept`

Acepta una invitación pendiente y agrega al destinatario como miembro regular del grupo.

## Req

- `POST /group-invitations/:id/accept`, protegido con JWT.
- `:id` es el UUID de la invitación. Sin body.
- El usuario destinatario se obtiene del JWT; no se puede aceptar la invitación de otra persona.

## Res

- `200`: `{"status":"accepted","message":"Invitación aceptada. Ahora eres miembro del grupo."}`.
- `404` si la invitación no existe, pertenece a otro usuario o ya fue respondida.
- `400` si el UUID es inválido; `401` si falta o no es válido el JWT; `500` ante errores de persistencia.

La respuesta y la creación de la membresía regular se realizan dentro de una transacción.

## Front

`GroupInvitationNotifications` llama `SocialService.acceptInvitation` desde el botón Aceptar y actualiza la lista de grupos al completarse.
