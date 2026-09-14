# Master prompt — Plataforma comunitaria de estudio

Este documento reemplaza la versión anterior del master prompt. Refleja la arquitectura
vigente tras eliminar el rol administrativo de profesor y adoptar un modelo local-first
con almacenamiento delegado a Google Drive. Úsalo como contexto base al trabajar con
herramientas de IA o al integrar a nuevos miembros del equipo.

---

## 1. Visión del producto

Aplicación de escritorio y móvil (Flutter, código único) enfocada en la **gestión
académica personal** y la **colaboración entre pares**. No existe jerarquía de profesor
ni curso oficial administrado por un tercero — todo grupo es un espacio de colaboración
entre estudiantes, gestionado por su(s) administrador(es). El producto es **local-first**:
los datos de un solo dueño (malla, materias, notas, apuntes) viven primero en el
dispositivo del usuario y sincronizan en segundo plano, funcionando sin conexión para el
uso cotidiano.

---

## 2. Foco del sprint actual: Notes + Social

Este sprint no toca Academic ni el Asistente de IA. El objetivo es dejar funcionando de
extremo a extremo: un usuario que crea y comparte apuntes, se organiza en grupos, y
colabora dentro de ellos (tareas, reuniones, chat).

### 2.1 Identity — lo que se completa este sprint

- Sesión completa: `POST /auth/refresh`, `POST /auth/logout`, rotación de refresh token.
- Perfil propio: `GET/PATCH /profile/me`.
- **Conexión a Google Drive y Google Calendar por OAuth**, con refresco automático de
  token y detección de revocación (`identity.oauth_connections`, ahora con providers
  `google_drive` y `google_calendar`).
- `GET /notes/{id}/access`: resuelve si el solicitante puede leer una nota y bajo qué
  `access_mode`.

### 2.2 Notes — núcleo completo de apuntes

**Decisión de arquitectura central de este sprint**: el contenido de un apunte **no vive
en Postgres**. Se almacena como un archivo Markdown en el Google Drive del propio autor
(`notes.notes.external_file_id`). Postgres guarda únicamente metadata: título, autor,
visibilidad, contador de likes, y el vínculo con la plantilla de compartición. El usuario
conecta Drive una sola vez al crear su cuenta y no vuelve a interactuar con Drive
directamente dentro de la app.

- CRUD de apuntes (`POST/PATCH/DELETE/GET /notes`), con manejo explícito de "nota no
  disponible" cuando falla la lectura desde Drive (permiso revocado, o el autor ya no
  existe en el sistema).
- Adjuntos (imágenes/PDF) también alojados en Drive, con distinción `is_inline` (embebido
  en el Markdown) vs. adjunto suelto.
- Guardar por referencia (`POST /notes/{id}/save`) y copiar/fork (`POST /notes/{id}/copy`,
  clona el archivo en el Drive de quien copia).
- Likes (`POST/DELETE /notes/{id}/like`), con `likes_count` desnormalizado y actualizado
  transaccionalmente.
- **Compartir con grupo** (`POST /notes/{id}/share`): admite dos modos de acceso —
  `link` (cualquiera con el enlace puede ver) o `restricted` (permiso nominal por email a
  cada miembro del grupo). Al compartir, se resuelve una sola vez si el autor es admin del
  grupo (`is_admin_note`), evitando cruzar al schema de Social en cada lectura posterior.
- Moderación: un admin del grupo puede retirar un apunte compartido
  (`DELETE /notes/shared/{id}`) sin borrar el original.
- Listado de apuntes de un grupo (`GET /groups/{id}/notes`), ordenado por admin primero,
  luego likes, luego seguidores del autor, luego fecha — con índice compuesto que cubre el
  filtro principal y paginación por cursor donde el orden lo permite.

### 2.3 Social — núcleo de grupos

- Sin distinción de tipo de grupo: se eliminó el campo `type` (antes `study`/
  `teacher_class`). Todo grupo es igual estructuralmente.
- Roles simplificados a **`admin`/`member`** — ya no existe el rol `teacher` ni `student`
  a nivel de membresía.
- Ciclo de vida completo: crear, unirse (`invite_token` regenerable), expulsar, banear,
  cambiar rol, transferir administración, abandonar (con opción de limpiar los apuntes
  propios compartidos en ese grupo al salir), y **sucesión automática de admin** si quien
  elimina su cuenta es el único administrador de un grupo.

### 2.4 Social — colaboración e integraciones externas

- Lista de tareas simple (`todo`) y hoja de sprint (`sprint-sheet`, con horas por día),
  ambas con validación de que el asignado sea miembro del grupo.
- Reuniones: creación local síncrona y obligatoria; sincronización con Google Calendar y
  notificación a Discord/Stream **asíncronas y de mejor esfuerzo** — un fallo en la
  integración externa nunca revierte la reunión ya creada.
- **Doble integración de comunicación externa, con roles distintos**:
  - **Discord**: servidor creado manualmente por el grupo, configurado con nombre, link de
    invitación y webhook opcional — usado para notificaciones (reuniones) y como espacio
    de comunidad más amplio.
  - **Stream** (getstream.io): canal de chat creado automáticamente al crear el grupo,
    integrado dentro de la propia app — es el chat in-app real, a diferencia de Discord
    que vive fuera de la aplicación.

---

## 3. Arquitectura

- Microservicios por dominio: **Identity, Notes, Social, Academic** (Academic no se toca
  este sprint), más un servicio de **Asistente de IA** en Python.
- Patrón por servicio: Handler → Service → Repository.
- Comunicación síncrona entre servicios: gRPC. Sin message broker (se evaluó Redis
  Pub/Sub y se descartó — el único caso de uso que lo habría justificado, un chat interno
  con WebSockets, ya no existe al resolver el chat con Stream/Discord).
- Una sola instancia de PostgreSQL (Supabase), un schema por servicio, sin foreign keys
  físicas cruzando schemas — solo referencias lógicas resueltas vía gRPC.
- **Cliente local-first**: cada dispositivo mantiene una base de datos local (Drift/SQLite)
  para los datos de un solo dueño (Academic personal, apuntes, perfil). La colaboración en
  grupo (chat, sprint sheet, reuniones) requiere conexión, al ser de múltiples editores.
- **Sin almacenamiento propio de archivos**: todo el contenido de apuntes y sus adjuntos
  vive en el Google Drive del usuario que los creó. El backend nunca toca los bytes del
  archivo, solo su metadata y su ID externo.

## 4. Stack tecnológico

| Capa | Tecnología | Notas |
|---|---|---|
| Frontend | Flutter | Un solo código para desktop y mobile |
| Almacenamiento local | Drift (SQLite) | Base del modelo local-first |
| HTTP framework | Gin | Todos los servicios Go |
| Acceso a datos | sqlc + pgx | SQL explícito, sin ORM |
| Base de datos | PostgreSQL (Supabase) | Un schema por servicio |
| Auth | golang-jwt + bcrypt | Access token corto + refresh token largo revocable |
| Comunicación entre servicios | gRPC | Sin message broker |
| Almacenamiento de contenido | Google Drive API (scope `drive.file`) | El contenido del apunte es un archivo Markdown |
| Reuniones | Google Calendar API | Sincronización asíncrona, best-effort |
| Chat in-app | Stream (getstream.io) | Canal automático por grupo, token emitido por el backend |
| Chat externo / comunidad | Discord (webhook + link manual) | Sin bot, sin automatización de servidor |
| IA (asistente) | API externa (Claude/OpenAI) vía FastAPI | Contexto limitado a apuntes propios del usuario |

## 5. Épicas fuera del sprint actual (referencia, no en desarrollo ahora)

- **Academic**: malla curricular, materias, horario, evaluaciones, promedio, asistencia —
  gestión 100% personal, sin ningún componente administrado por un tercero. Se retoma en
  un sprint posterior.
- **Asistente de IA**: consulta sobre apuntes propios. Su implementación deberá leer el
  contenido directo desde Drive (ya no desde un campo de texto en Postgres), lo cual es un
  ajuste menor pendiente de tener presente cuando se aborde.

## 6. Roles y permisos (vigente)

- No existe un rol global de usuario (se eliminó la distinción `student`/`teacher` a nivel
  de cuenta).
- Dentro de un grupo, el rol es `admin` o `member`. Solo `admin` puede: expulsar/banear,
  cambiar roles, transferir administración, configurar Discord, y activar restricciones
  como `notes_restricted_to_staff` o la limitación de quién puede agendar reuniones.
- Un admin no puede expulsar a otro admin. El último admin de un grupo no puede
  abandonarlo sin transferir el rol antes; al eliminar su cuenta, se promueve
  automáticamente al miembro más antiguo.

## 7. No-funcionales relevantes a este sprint

- Toda integración externa (Drive, Calendar, Discord, Stream) debe fallar de forma
  aislada: un error ahí nunca debe impedir ni revertir la operación principal que la
  origina (crear un apunte, agendar una reunión).
- El cliente debe operar sin conexión para los datos de un solo dueño, sincronizando
  cuando la hay.
- Ninguna llamada a la API de Drive/Calendar debe bloquear la respuesta al usuario más
  allá de lo estrictamente necesario — las sincronizaciones de contenido van en segundo
  plano.

## 8. Diseño futuro (no implementado, documentado para no perderlo)

- **Feed general de descubrimiento**: cursos/apuntes públicos de la comunidad, con
  búsqueda difusa y actualización periódica (no en vivo). Sigue diferido.
- **Extensión de `access_mode: restricted`**: hoy no otorga acceso automático a quien se
  une a un grupo después de que una nota ya fue compartida en ese modo — pendiente de
  resolver si se vuelve un problema real de uso.
- **Cifrado de apuntes privados** antes de subirlos a Drive, para que ni Google pueda leer
  su contenido en texto plano.

## 9. Próximo paso

Cerrar las dos decisiones pendientes del contrato de API de este sprint (si Drive y
Calendar comparten un solo flujo de consentimiento OAuth, y en qué servicio vive la
emisión del token de Stream), y comenzar la implementación siguiendo el backlog y el
contrato de API ya definidos para Notes y Social.