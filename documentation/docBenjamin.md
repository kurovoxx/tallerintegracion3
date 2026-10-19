# Benjamín Rojas

## 3_3_6 — Puertos gRPC Social en Docker y Kubernetes

- Social escucha HTTP en `8083` y gRPC en `9083`.
- `docker-compose.yml` publica `9083:9083` una sola vez y configura
  `SOCIAL_GRPC_PORT=9083`. La imagen declara ambos puertos en `EXPOSE`.
- `k8s/manifests.yaml` ya declara `containerPort: 9083`,
  `SOCIAL_GRPC_PORT=9083` y el puerto `grpc` del Service `social`
  (`port: 9083`, `targetPort: 9083`). No fue necesario modificarlo.
- Dentro de Compose y Kubernetes, los clientes usan `social:9083`;
  Notes ya tiene `SOCIAL_GRPC_ADDR` configurada con esa dirección.
- Las llamadas requieren metadata `x-internal-key` con la misma
  `INTERNAL_API_KEY` del servidor. Kubernetes la obtiene de `ti3-secrets`.
- Para acceder desde el equipo al clúster durante una prueba:
  `kubectl -n student-brojas port-forward svc/social 9083:9083`.

## 3_3_11 — CI de Social

Se amplió `.github/workflows/test-social-groups.yml`, conservando el nombre
del workflow y del job para mantener los checks existentes.

Se ejecuta en push, pull request o manualmente. Sus filtros incluyen Social,
los protos Social/Auth/Common, el esquema de PostgreSQL, Compose, los
manifiestos de Kubernetes y el propio workflow.

El job usa la versión de Go de `back/social/go.mod` y PostgreSQL 16 en una
base temporal `social_test`. Inicializa `docker/postgres/init.sql`, ejecuta
`go vet ./...`, `go build ./...` y la suite completa:

```sh
cd back/social
go test ./... -race -p 1 -count=1 -timeout=5m -json
```

Incluye tests unitarios, gRPC con bufconn, integración y E2E existentes de
Social. Falla ante errores o tests omitidos, y exige que
`TestGroupsPostgresE2E_MembershipLifecycle` produzca un PASS. Los paquetes
sin archivos de pruebas no se consideran tests omitidos. La salida JSON
queda visible en el log de Actions, incluso si falla una prueba.

Para integración local se necesita `SOCIAL_TEST_DATABASE_URL` apuntando a
una base exclusiva de pruebas con el esquema inicializado. Sin esa variable,
las pruebas que requieren PostgreSQL se omiten. Esta suite no reemplaza el
E2E entre procesos Social↔Notes↔Identity de la tarea 3_3_5.

### Verificación local

- `go test ./... -count=1`, `go vet ./...` y `go build ./...`: correctos.
- Las pruebas con PostgreSQL se omitieron porque no hay una base de pruebas
  configurada. Docker y kubectl no están disponibles en este entorno;
  la validación con PostgreSQL y detector de carreras se realizó en Actions.

### 3_3_11 — Validación en GitHub Actions

La suite completa pasó sobre el commit `c1c1124` en `Development`:
[ejecución de test-social-groups](https://github.com/kurovoxx/tallerintegracion3/actions/runs/38656300385).

La primera ejecución detectó carreras en las pruebas de notificaciones de
Calendar, invitados, Discord y Stream. Se corrigieron los tests para esperar
el fin del notifier mediante un canal antes de leer los mocks, manteniendo
la ejecución real en segundo plano y el detector de carreras habilitado.

La ejecución valida compilación, análisis estático, pruebas gRPC, integración
y E2E existentes con PostgreSQL 16. La tarea 3_3_11 queda validada; el E2E
entre servicios desplegados sigue correspondiendo a 3_3_5.

## 3_3_5 — E2E Social ↔ Notes ↔ Identity

Se agregó `back/social/test/cross_service_e2e_test.go`. Compila y arranca los
tres servidores de producción como procesos separados, con HTTP, gRPC y
PostgreSQL reales. No importa código de otros servicios ni carga los `.env`
del repositorio. Drive, Calendar y Stream usan sus modos mock, sin cuentas
ni credenciales reales de proveedores externos.

El harness recibe `CROSS_SERVICE_TEST_DATABASE_URL`, exige que el nombre de
la base termine en `_test` y crea una base hija única `social_cross_e2e_*`.
Inicializa esa hija con `docker/postgres/init.sql` y la elimina al terminar,
después de detener los procesos. El usuario de PostgreSQL necesita permiso
para crear bases; no se reinicializa ni elimina la base indicada en la URL.

Casos cubiertos:

- Registro/login real en Identity; Social recibe sus JWT y resuelve los
  nombres y correos de los miembros mediante Identity gRPC.
- Compartir apuntes privados con acceso `restricted` mediante Notes, que
  consulta la membresía real en Social por gRPC.
- Salida, expulsión y baneo con limpieza de los apuntes del afectado;
  conservación de los shares de otros autores y del mismo apunte en otro
  grupo; rechazo de acceso de externos y de miembros que ya salieron.
- Reingreso permitido después de salir/ser expulsado, pero denegado si
  existe un baneo; salida sin limpieza y sucesión del administrador.
- `AccountDeletionCleanup` autenticado, sucesión, limpieza e idempotencia.
  Este caso llama al RPC Social: no equivale a un borrado iniciado desde
  Identity, cuyo endpoint HTTP de eliminación de cuenta no está conectado
  en el servidor actual.

El fallback HTTP de limpieza apunta a una ruta inexistente durante la
prueba: si falla el gRPC Social → Notes, no puede ocultarlo el transporte
legacy. Las aserciones verifican metadatos de shares y acceso HTTP; los
permisos reales de Google Drive requieren una prueba separada con OAuth.

La suite tiene build tag `crossservice`: no se omite silenciosamente si
falta configuración y no cambia la ejecución normal de `go test ./...`.
Desde `back/social`, con una base dedicada PostgreSQL disponible:

```powershell
$env:CROSS_SERVICE_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/social_test?sslmode=disable'
go test -tags=crossservice ./test -count=1 -timeout=12m -v
```

Se agregó `.github/workflows/test-cross-services.yml`, que ejecuta la suite
con PostgreSQL 16 temporal al cambiar Auth, Notes o Social, o manualmente.

## 3_3_9 — Revisión de integridad de Social

Se revisaron autorización de grupos, invitaciones, sucesión y limpieza.
Se reprodujeron cuatro fallos y se corrigieron con regresiones que fallaban
antes del cambio y pasan después:

1. **Baneo concurrente con reingreso:** Join validaba el baneo antes de
   esperar el commit de Ban y podía insertar una membresía nueva después.
   Ahora bloquea primero la fila del grupo, igual que las demás operaciones
   administrativas, y luego valida el baneo y la invitación.
2. **Exadministrador que regenera una invitación:** la autorización podía
   evaluarse antes de confirmar una transferencia de administración.
   RegenerateInviteToken ahora usa `requireAdminTx` bajo el lock del grupo.
3. **Borrado de cuenta con fallo parcial:** el repositorio confirma cada
   grupo por separado. El servicio descartaba los resultados confirmados
   al recibir un error posterior y no pedía su limpieza en Notes; un
   reintento tampoco encontraba esas membresías. Ahora procesa los grupos
   confirmados y conserva el error para permitir reintentar los restantes.
4. **Fallo de base de datos confundido con grupo inexistente:** ListMembers
   ignoraba el error de la consulta de existencia. Ahora lo propaga y
   comprueba también errores durante el recorrido de filas.

Las carreras se prueban con transacciones PostgreSQL reales, pausando la
operación antes del commit y comprobando en `pg_stat_activity` que la otra
operación espera el lock. No dependen de asumir un orden por tiempos de
espera fijos. La limpieza remota conserva su diseño best-effort existente.

## 3_3_12 — Pruebas de endpoints Social

Se ampliaron pruebas de JWT real, identidad obtenida del token, rechazo de
suplantación de dueño y de privilegios globales, validación de UUID y
autenticación de los cinco RPC internos. Se comprueba que los errores de
persistencia no filtren detalles y que una falla de Identity no entregue
una lista parcial de correos.

Se añadieron unidades para GET horas y GET Discord, que antes no tenían
cobertura: autorización, entradas inválidas, recursos inexistentes,
respuestas vacías, actualización diaria y totales, configuración y webhook
opcional. El filtrado de horas por rango se comprueba con PostgreSQL real,
porque el store en memoria no implementa ese filtro.

`back/social/test/social_routes_e2e_test.go` complementa el E2E anterior
con el recorrido del resto de rutas del servidor real, sus efectos
persistidos y rechazos seleccionados de autorización/aislamiento:

| Familia | Rutas HTTP | Comprobación E2E |
|---|---:|---|
| Grupos | 11 | Membresía, invitaciones, roles, transferencia y sucesión |
| TODO/kanban | 4 | Crear, actualizar, filtrar, eliminar y aislar grupos |
| Hojas sprint | 4 | Crear, listar, actualizar, eliminar y aislar grupos |
| Tareas sprint | 4 | CRUD, asignación, filtros y aislamiento |
| Horas | 2 | Alta, actualización por fecha, total y rango de fechas |
| Reuniones | 2 | Crear y consultar reuniones próximas |
| Notificaciones | 2 | Destinatarios, lectura persistida y rechazo de avisos ajenos |
| Discord | 2 | Guardar/consultar configuración y permisos |
| Stream | 1 | Token/canal para miembros y rechazo de externos |
| Vistas | 2 | Overview y workspace reflejan recursos persistidos |
| HTTP interno | 2 | Miembros/correos y exigencia de clave interna |
| Salud | 2 | HTTP y conexión a la base hija de pruebas |
| **Total HTTP** | **38** | **Todos los endpoints registrados en Social** |

Los cinco RPC gRPC (`IsMember`, `IsAdmin`, `GetMembers`, `GetMemberEmails`,
`AccountDeletionCleanup`) tienen pruebas bufconn y llamadas sobre TCP real
en la suite cruzada. Cubrir todas las rutas no significa cubrir todas las
combinaciones de entradas ni alcanzar 100% de cobertura de líneas.

### Verificación del 19 de octubre de 2026

- Suite completa en Windows con `SOCIAL_TEST_DATABASE_URL` contra
  PostgreSQL 16 temporal: **338 casos aprobados, 0 omitidos, 0 fallos**
  (el conteo incluye subtests).
- La misma suite en Linux con Go 1.26 y `-race -p 1 -count=1`:
  **338 casos aprobados, 0 omitidos, 0 fallos**, sin carreras detectadas.
- E2E cruzado completo en Windows: **PASS**, 12,002 s.
- E2E cruzado completo en Linux con Go 1.26: **PASS**, 51,905 s,
  incluyendo compilación y arranque de los tres servidores.
- `go vet -tags=crossservice ./...`, `go build ./...` y
  `git diff --check`: correctos.
- Los workflows Social y E2E cruzado pasan `actionlint`.
- No se ejecutó el nuevo workflow en GitHub Actions ni se probó el clúster;
  la ejecución automática ocurrirá después de subir los cambios.

### Comprobación del despliegue Kubernetes (19 de octubre de 2026)

Se consultó el contexto `estudiante-brojas`, namespace `student-brojas`.
Auth, Notes, Social, Academic, Front y PostgreSQL están `Running`, con sus
deployments disponibles 1/1. Los pods actuales arrancaron el 17 de octubre;
las correcciones locales de esta sesión todavía no están publicadas allí.

Comprobaciones iniciales sobre los servicios desplegados:

- Los cuatro backends responden `200` en `/health` desde la red interna y
  mediante el gateway del front por un túnel Kubernetes local.
- Auth y Social responden `200` en `/health/db`. Auth informa que su
  servidor gRPC está sirviendo. Notes no publica `/health/db` (`404`);
  ese `404` por sí solo no significa que la base esté caída.
- Las rutas protegidas consultadas sin JWT responden `401`.
- `auth:9080` y `notes:9082` no aceptan conexiones mediante sus Services:
  solo publican HTTP `8080` y `8082`. Sus pods sí aceptan conexiones gRPC
  directas en `9080` y `9082`, respectivamente. `social:9083` sí funciona.
  Los manifiestos locales ya contienen los puertos gRPC que faltaban en esos
  dos Services. Esta era la configuración antes de la corrección indicada
  más abajo.
- El HTTPS público presenta `Kubernetes Ingress Controller Fake
  Certificate`, autofirmado y emitido para `ingress.local`, no para
  `ti3-brojas.dev.censei.cl`. La validación TLS normal falla; las pruebas
  autenticadas usan el túnel Kubernetes, sin desactivar la validación TLS
  para enviar credenciales al dominio público.
- La cuenta de prueba indicada por el usuario existe, pero en la consulta
  inicial no tenía conexiones vigentes `google_drive` ni `google_calendar`.
  No se usaron cuentas OAuth de otros usuarios.

La configuración desplegada tiene Notes en `STORAGE_MODE=drive`, Social en
`STREAM_MODE=real` y Calendar en `mock`. Una prueba local aprobada con mocks
no demuestra por sí misma que esos proveedores reales funcionen.

Se agregó `back/social/test/cluster_smoke_test.go` (build tag `clustersmoke`)
para comprobar los servicios ya desplegados, mediante port-forward. La
prueba exige confirmación explícita de namespace y conexiones loopback;
registra tres cuentas aleatorias `@example.invalid` y usa grupos/notas de
UUID exacto. No inicializa ni recrea la base compartida. Los grupos se
insertan como fixtures para evitar crear canales Stream; las notas no
tienen archivo externo, por lo que no se accede a Google Drive. Al terminar
se eliminan y verifican únicamente los registros de esta ejecución.

Primera ejecución: **5 escenarios PASS, 4 FAIL, 0 omitidos**, 86,449 s.
Funcionaron salud/autenticación, directorio y membresía RPC, invitaciones,
roles, transferencia, sucesión sin limpieza, TODO, sprint, horas,
configuración Discord y vistas. Fallaron las limpiezas de shares después
de leave/kick/ban y `AccountDeletionCleanup`: quedaban metadatos compartidos.
Los fixtures y cuentas temporales fueron retirados al terminar.

Con autorización explícita del usuario se añadieron únicamente los puertos
gRPC internos de los Services `auth` (`9080`) y `notes` (`9082`), manteniendo
sus puertos HTTP. Se validaron antes con dry-run del servidor. Los pods no
se reiniciaron. Desde Social, las conexiones a ambos Services pasaron de
rechazadas a aceptadas. Los manifiestos versionados ya incluyen ambos
puertos, por lo que no fue necesario cambiarlos.

Segunda ejecución sobre el mismo despliegue, después de añadir los puertos:
**9 escenarios PASS, 0 FAIL, 0 omitidos**, 25,748 s. Las cuatro limpiezas de
shares que fallaban pasaron. La limpieza de cuenta comprobó además sucesión,
eliminación del grupo cuyo último miembro se retira e idempotencia. Los
datos de prueba se retiraron y no hubo reinicios adicionales de pods.

Esta suite valida registro/login, permisos de grupos, los cinco RPC Social,
compartir/listar/limpiar metadatos Notes, TODO, hojas y tareas sprint, horas,
configuración Discord y vistas. POST de grupos se comprueba sólo con una
entrada inválida y Stream sólo con un usuario externo denegado. No se
ejecutan reuniones ni notificaciones reales ni se atribuye este resultado
a las correcciones de código locales todavía sin desplegar. No sustituye
el E2E local de las 38 rutas con proveedores mock.

Por indicación del usuario, Google Drive real queda pendiente. Tampoco se
prueban Calendar real (el despliegue está en mock), mensajes reales de
Stream/Discord ni el borrado de cuenta iniciado desde Identity.
