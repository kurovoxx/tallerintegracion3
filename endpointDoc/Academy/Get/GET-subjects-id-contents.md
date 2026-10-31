# GET /subjects/{id}/contents

## Req
- Método: `GET`, ruta: `/subjects/:id/contents` (protegida, `back/infacadem/cmd/server/main.go:175`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[{"id":"...","subject_id":"...","title":"...","body":null,"position":0}]}` (orden `position, id`; `[]` nunca `null`).
- `400/401/404`: `invalid_uuid` / auth / materia ajena.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:678` (`listContents`)
  - Pantalla: `front/lib/features/academic/courses_screen.dart:480` (cargar unidades)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/contents_handler.go` (`ListBySubject`) → `service` → `repository` (`ListBySubject` con check de propiedad).

## Tests
- `back/infacadem/internal/handler/http/content_handler_test.go`, `back/infacadem/internal/service/content_service_test.go`.
