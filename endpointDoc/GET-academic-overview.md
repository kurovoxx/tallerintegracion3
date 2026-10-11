# GET /academic/overview

## Req
- Método: `GET`, ruta: `/academic/overview` (protegida, `back/infacadem/cmd/server/main.go:165`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"curricula":[{"id":"...","name":"Malla A","subjects_count":2,"approved_count":1,"in_progress_count":1}],"stats":{"curricula_count":1,"subjects_count":2}}` (solo propias, orden creación desc heredado del listado; `curricula: []` y stats en cero si vacía).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.

## Flujo
- `back/infacadem/internal/handler/http/overview_handler.go` (`Overview`) → `back/infacadem/internal/service/overview_service.go` (`OverviewService.Get`: `CurriculumService.List` + `SubjectRepository.CountsByCurricula`, 1 query de mallas + 1 `GROUP BY` con `FILTER`, sin N+1).

## Tests
- `back/infacadem/internal/handler/http/overview_handler_test.go`: `TestOverview_HTTP` (JSON exacto, aislamiento cross-user sin rastro, 401).
- `back/infacadem/internal/service/overview_service_test.go`: `TestOverview_Vacia` (listas `[]`, stats cero), `TestOverview_Conteos`.
