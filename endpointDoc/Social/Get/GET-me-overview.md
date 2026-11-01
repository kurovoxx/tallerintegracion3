# GET /me/overview

Vista principal del usuario: sus grupos (tarjetas), barra lateral y contadores (microservicio `social`).

## Req
- Método: `GET`, ruta: `/me/overview` (protegida con JWT `Bearer`; `back/social/internal/handler/http/view_handler.go:24`).
- Headers: `Authorization: Bearer <access_token>`. Sin parámetros: el usuario sale del token.

## Res
- `200`:
  ```json
  {
    "user_id": "<uuid>",
    "sidebar": {"groups": [{"group_id","name","role"}]},
    "groups": [{"group_id","name","description?","role","member_count","joined_at"}],
    "stats": {"groups_count": 2, "admin_groups_count": 1}
  }
  ```
  Listas vacías salen como `[]`.
- `401` `{"error":"No autorizado"}`. `500`: error de base de datos.

## Flujo
- `ViewHandler.MyOverview` (`internal/handler/http/view_handler.go:29`) → `GroupService.MyOverview` (`internal/service/group_service.go:235`: arma tarjetas y cuenta grupos donde el rol es `admin`) → `GroupRepository.ListMyGroupsDetailed`.

## Tests
- `internal/handler/http/view_handler_test.go`: `TestOverviewEmptyUser`, `TestOverviewPayload`, `TestViewRoutesRequireAuth`.
