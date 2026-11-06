# `POST /groups/:id/transfer-admin`


## Req

- `POST /groups/:id/transfer-admin` con JWT: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id UUID. JSON requerido {"new_admin_user_id":"<uuid>"}.
- El actor debe ser admin; el usuario destino debe ser miembro elegible del grupo.

## Res

- 200 sin body.
- El destino pasa a admin y el actor pasa a member, en una transacción.
- 400 si falta id/body o la solicitud no es válida; 401 sesión inválida; 403 actor sin permisos; 404 target/grupo no encontrado; 500 error de storage. respondGroupError usa {"error":"mensaje"}; auth previo usa el envelope común.
- Esta operación no cambia owner_user_id; la sucesión al salir/borrar cuenta es un flujo distinto.

## Invocación (Front / Otros Servicios)

- `_GroupMembersTabState._confirmRole` muestra confirmación y llama a `transferAdmin` (`front/lib/features/groups/group_members_tab.dart:230-280`).
- `SocialService.transferAdmin` envía POST y notifica que el rol del usuario actual cambió (`front/lib/core/services/social_service.dart:291-314`).
- No se encontró consumidor backend.

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` bajo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.TransferAdmin` valida el nuevo administrador y toma al solicitante del JWT (`back/social/internal/handler/http/group_handler.go:262-278`).
3. `GroupService.TransferAdmin` protege al usuario destino durante borrado; `GroupRepository.TransferAdmin` verifica que sea miembro y actualiza ambos roles dentro de una transacción (`back/social/internal/service/group_service.go:568-575`; `back/social/internal/repository/group_repository.go:383-421`).
4. El handler devuelve `200` sin body, y `_confirmRole` recarga la lista tras `SocialService.transferAdmin` (`back/social/internal/handler/http/group_handler.go:273-278`; `front/lib/core/services/social_service.dart:291-314`; `front/lib/features/groups/group_members_tab.dart:230-280`).

## Front
- Cliente: `SocialService.transferAdmin` (`front/lib/core/services/social_service.dart:294`); envía `{new_admin_user_id}`; espera `200`.
- Pantalla: `_GroupMembersTabState._confirmRole(action: transfer)` (`front/lib/features/groups/group_members_tab.dart:258`), acción TRANSFERIR ADMIN (diálogo en rojo: el usuario pasa a miembro).
- Efecto: `notifyGroupsChanged()` (cambia el rol propio en la lista/sidebar), snackbar `Administración transferida a <nombre>.` y recarga de integrantes.

## Tests

- `back/social/internal/handler/http/group_handler_test.go:280`, `TestTransferAdminContract`.
- `back/social/internal/service/group_service_test.go:872`, `TestLeaveSuccessionOrderAndOwner` documenta como contraste la sucesión por salida.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` recorre el ciclo.
