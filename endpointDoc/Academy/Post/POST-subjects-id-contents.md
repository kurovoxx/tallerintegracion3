# POST /subjects/{id}/contents

## Req
- Método: `POST`, ruta: `/subjects/:id/contents` (protegida, `back/infacadem/cmd/server/main.go:174`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"title":"Unidad 1","body":"...","position":0}` (`title` 1–200 requerido; `body`/`position` opcionales).

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_uuid","message":"..."}}`.
- `401` / `404` (materia ajena/inexistente): códigos habituales.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:693` (`createContent`)
  - Pantalla: `front/lib/features/academic/courses_screen.dart:495` (crear unidad)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/contents_handler.go` (`Create`) → `service` (valida `title`) → `repository` (`Create`, `INSERT..SELECT..WHERE EXISTS` propiedad).

## Tests
- `back/infacadem/internal/handler/http/content_handler_test.go` (feliz + validaciones + cross-user).
- `back/infacadem/internal/service/content_service_test.go` + integración (cascada subject→contents).
