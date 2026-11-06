# `POST /groups/:id/members/:user_id/ban`


## Req

- `POST /groups/:id/members/:user_id/ban` con JWT: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id y :user_id son UUID. Solo admin. Body opcional {"cleanup_shared_notes":true|false}; default false.
- La identidad del admin sale del JWT; el objetivo se especifica en path.

## Res

- 204 sin body si se retiró y bloqueó el miembro.
- 400 para target self o restricciones de rol; 401 sin JWT; 403 sin rol admin; 404 si no pertenece/no se encuentra el grupo. respondGroupError devuelve {"error":"mensaje"}; auth previo usa {"error":{"code":"...","message":"..."}}.
- Inserta ban/marca de no reingreso en Social. Stream y Notes se solicitan best-effort; 204 no prueba confirmación externa. cleanup_shared_notes=true ejecuta cleanup de Notes en background.

## Invocación (Front / Otros Servicios)

- `_GroupMembersTabState._confirmModeration` confirma la acción y el alcance opcional de limpieza de shares (`front/lib/features/groups/group_members_tab.dart:175-226`).
- `SocialService.banMember` envía POST y acepta `204` (`front/lib/core/services/social_service.dart:239-261`).
- No se encontró invocación de producción desde otro microservicio.

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` bajo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.BanMember` toma actor, objetivo y body opcional, y conserva el JWT para Notes (`back/social/internal/handler/http/group_handler.go:217-234`).
3. `GroupService.BanMember` protege la operación con `withTargetAccount`, persiste expulsión/ban vía `GroupRepository.BanMember` y agenda la retirada Stream (`back/social/internal/service/group_service.go:541-556`; `back/social/internal/repository/group_repository.go:318-350`; `back/social/internal/service/group_service.go:133-151`).
4. Con `cleanup_shared_notes=true`, `unshareNotes` ejecuta `NotesService/UnshareAll` por gRPC y usa HTTP legacy como fallback; es best-effort y en segundo plano. El handler responde `204` sin esperar esos proveedores (`back/social/internal/service/group_service.go:595-619`; `back/social/internal/handler/http/group_handler.go:228-234`).

## Front
- Cliente: `SocialService.banMember` (`front/lib/core/services/social_service.dart:239`); envía `{cleanup_shared_notes}`; acepta `204` o `200`.
- Pantalla: `_GroupMembersTabState._confirmModeration(ban: true)` (`front/lib/features/groups/group_members_tab.dart:215`), acción BANEAR; el diálogo avisa que no podrá reingresar ni con enlace válido.
- Efecto: snackbar `Integrante baneado.` y recarga de la lista.

## Tests

- `back/social/internal/handler/http/group_handler_test.go:235`, `TestKickAndBanContract`.
- `back/social/internal/service/group_isolation_test.go:48`, `TestBanIsScopedToTheGroup`; :56 verifica que el baneado pierde acceso.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` recorre el ciclo HTTP en PostgreSQL.
