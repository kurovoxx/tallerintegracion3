# Documentación — Agustín Vega / Auth, Drive e Integración

Este documento resume las tareas realizadas por Agustín Vega durante el Sprint 1, agrupándolas por área para evitar repetir cada actividad de manera aislada.

El trabajo se concentró principalmente en autenticación y manejo de sesiones, perfil de usuario, integración con Google Drive, permisos de acceso a Notes, integración de vistas del frontend, revisión técnica y actualización de documentación del proyecto.

---

## 0. Alcance y estado actual

Las tareas asignadas abarcan los identificadores `2_4_1` a `2_4_23`.

A nivel general, el trabajo puede agruparse en las siguientes áreas:

- Renovación y cierre de sesiones.
- Perfil del usuario.
- Conexión y mantenimiento de Google Drive.
- Acceso a Notes y permisos restringidos.
- Reconciliación entre Notes y Google Drive.
- Integración del frontend con servicios reales.
- Documentación de endpoints y lógica de negocio.
- Revisión de integridad de código de otros módulos.
- Actualización de reglas de negocio.
- Actualización del diagrama de casos de uso.
- Investigación de documentación de otros integrantes.

### Estado resumido de las tareas

| Tareas | Área | Estado |
|---|---|---|
| `2_4_1` a `2_4_5` | Refresh de sesión y rotación de tokens | Completado |
| `2_4_6` a `2_4_8` | Logout y revocación de sesión | Completado |
| `2_4_9` a `2_4_10` | Perfil y visibilidad | Completado |
| `2_4_11` a `2_4_13` | Google Drive OAuth, refresh y revocación | Completado |
| `2_4_14` a `2_4_16` | Acceso a Notes y permisos restricted | Completado / colaborativo |
| `2_4_17` y `2_4_18` | Integración de vistas del frontend | Completado |
| `2_4_19` | Documentación de endpoints y lógica implementada | Completado |
| `2_4_20` | Revisión de integridad del código de Martín | Completado |
| `2_4_21` | Investigación de documentación de Miguel, Héctor y Benjamín | En curso |
| `2_4_22` | Actualización de reglas de negocio | Actualizado |
| `2_4_23` | Actualización del diagrama de casos de uso | Actualizado |

---

## 1. Autenticación y manejo de sesiones

Una de las áreas principales de mi trabajo fue completar el manejo de sesiones del servicio de autenticación.

### 1.1 `POST /auth/refresh`

Implementé el endpoint encargado de renovar una sesión utilizando un `refresh_token` válido.

El flujo permite obtener un nuevo `access_token` sin solicitar nuevamente correo y contraseña al usuario.

La validación comprueba que el refresh token:

- exista;
- corresponda con el hash almacenado;
- no se encuentre revocado;
- no se encuentre expirado.

Los refresh tokens se almacenan mediante hash, por lo que el valor original no queda guardado directamente en la base de datos.

### 1.2 Emisión del nuevo access token

Una vez validado el refresh token, el sistema genera un nuevo JWT para el usuario correspondiente.

El token contiene la identidad necesaria para autorizar posteriores solicitudes, sin utilizar un rol global dentro del JWT.

### 1.3 Rotación del refresh token

Cada vez que un refresh token se utiliza correctamente, el token anterior se revoca y se genera uno nuevo.

La rotación evita que un mismo refresh token pueda reutilizarse varias veces.

La operación se realiza de forma transaccional para impedir que solicitudes simultáneas generen más de un token válido a partir de la misma sesión.

### 1.4 Tests de refresh

Se implementaron pruebas para los principales escenarios:

- refresh válido;
- refresh inexistente;
- refresh revocado;
- refresh expirado;
- reutilización del token anterior;
- body inválido.

Las pruebas verifican tanto el comportamiento del servicio como la respuesta HTTP.

---

## 2. Logout y revocación de sesión

### 2.1 `POST /auth/logout`

Implementé el endpoint encargado de cerrar una sesión específica.

El logout utiliza el refresh token de la sesión que se desea cerrar y marca su registro como revocado.

No elimina la fila de la base de datos, permitiendo conservar el estado de la sesión para control y trazabilidad.

### 2.2 Comportamiento idempotente

El logout fue diseñado para ser idempotente.

Esto significa que cerrar una sesión que ya se encontraba revocada no provoca un error funcional.

De la misma forma, la operación no revela innecesariamente si un token existía o no.

### 2.3 Tests de logout

Se cubrieron casos como:

- cierre de sesión exitoso;
- token ya revocado;
- token inexistente;
- token expirado;
- body vacío o mal formado;
- comprobación de que logout no emite nuevos tokens.

---

## 3. Perfil de usuario

### 3.1 `GET /profile/me`

Implementé el endpoint que permite consultar el perfil del usuario autenticado.

La identidad se obtiene desde el JWT de la sesión y no desde un `user_id` enviado libremente por el cliente.

La respuesta puede incluir información como:

- nombre visible;
- foto;
- teléfono;
- institución;
- descripción;
- visibilidad;
- email cuando está disponible.

### 3.2 `PATCH /profile/me`

También implementé la actualización parcial del perfil.

El usuario puede modificar únicamente los campos que envía, manteniendo sin cambios el resto de la información.

La operación siempre utiliza el usuario autenticado, evitando que el cliente pueda modificar el perfil de otra persona enviando otro identificador.

### 3.3 Visibilidad

La visibilidad del perfil acepta:

- `public`
- `private`

También se agregaron validaciones de longitud para distintos campos del perfil.

Antes de guardar los cambios se valida todo el request, evitando aplicar modificaciones parciales cuando alguno de los valores es inválido.

### 3.4 Tests

Se probaron escenarios de:

- consulta correcta;
- usuario no autenticado;
- perfil inexistente;
- actualización parcial;
- visibilidad inválida;
- campos demasiado largos;
- intento de enviar un `user_id` externo;
- request inválido sin modificación parcial.

---

## 4. Integración con Google Drive

Otra parte importante de mi trabajo fue completar la conexión entre la cuenta del usuario y Google Drive.

### 4.1 `POST /auth/google-drive/connect`

Implementé el endpoint utilizado para vincular una cuenta de Google Drive con la aplicación.

El frontend obtiene un código OAuth desde Google y el backend realiza el intercambio correspondiente.

La conexión se guarda en `identity.oauth_connections`.

La información privada de Google permanece en el backend y los tokens nunca se devuelven directamente al frontend.

Cuando se entrega un email esperado, se comprueba que la cuenta autorizada corresponda con ese usuario.

### 4.2 Estado de la conexión

También implementé la consulta del estado de Google Drive mediante:

`GET /auth/google-drive/status`

Este endpoint permite al frontend distinguir, entre otros casos:

- usuario conectado;
- usuario desconectado;
- usuario que necesita reconectar Drive.

### 4.3 Renovación automática del access token

El access token de Google tiene una duración limitada.

Implementé la lógica para comprobar su vigencia antes de utilizar Google Drive.

Si el token está vencido o se encuentra próximo a expirar, el backend utiliza el refresh token almacenado para solicitar un nuevo access token.

La implementación utiliza un margen aproximado de cinco minutos antes de la expiración.

### 4.4 Detección de revocación

Cuando Google informa que la autorización ya no es válida, la conexión se marca como revocada.

Esto permite que posteriormente el frontend muestre al usuario que debe reconectar su cuenta.

Un token que ya se encuentra marcado como revocado no provoca intentos innecesarios de conexión con Google.

### 4.5 Seguridad

En esta integración se mantuvieron varias reglas:

- los tokens de Google permanecen en el backend;
- los códigos OAuth no se registran en logs;
- los refresh tokens de Google no se exponen a la interfaz;
- los eventos de renovación se registran sin incluir credenciales.

### 4.6 Tests de Drive

Se documentaron y probaron casos como:

- conexión válida;
- token expirado y renovación correcta;
- autorización revocada;
- conexión previamente revocada;
- consulta del estado conectado/desconectado;
- desconexión;
- logs sin filtración de tokens.

Las pruebas utilizan mocks o stubs y no dependen de credenciales reales de Google.

---

## 5. Acceso a Notes y permisos de Google Drive

### 5.1 Aporte en `GET /notes/:id/access`

Este endpoint es colaborativo.

La base del endpoint pertenece al módulo Notes desarrollado principalmente por Héctor. Mi aporte se concentró en ampliar y endurecer la lógica de acceso, especialmente para los mecanismos relacionados con Google Drive.

La respuesta permite determinar cómo puede acceder el usuario a una nota y conocer el estado de sincronización con Drive.

Los modos considerados son:

- `owner`;
- `public`;
- `link`;
- `restricted`.

También se mantuvo el principio de no revelar innecesariamente si existe un recurso al que el usuario no tiene acceso.

### 5.2 Modo `restricted`

Implementé la lógica relacionada con permisos nominales de Google Drive para notas compartidas en modo restringido.

El flujo general es:

1. Resolver los miembros autorizados del grupo.
2. Obtener sus emails.
3. Crear permisos de lectura en Google Drive para esos correos.
4. Mantener en la aplicación el estado deseado de esos permisos.
5. Permitir reconciliar los permisos cuando ocurre un fallo parcial.

La intención es que el acceso restringido no se convierta en un enlace público, sino que quede limitado a las cuentas autorizadas.

### 5.3 Fallos parciales

Los errores temporales de Google Drive no deben provocar automáticamente la pérdida de una nota o de su estado interno.

Cuando una operación de permisos no puede completarse, su estado puede quedar pendiente o fallido para ser corregido posteriormente.

---

## 6. Reconciliación entre Notes y Google Drive

### `POST /notes/reconcile`

Posteriormente implementé el endpoint de reconciliación de Notes con Google Drive.

Su objetivo es comprobar diferencias entre lo que la aplicación espera encontrar y el estado real de los archivos externos.

Una ausencia aparente no provoca inmediatamente la eliminación de información.

Primero se diferencia entre:

- archivo confirmado como inexistente;
- archivo enviado a la papelera;
- error temporal de red;
- error de autenticación;
- falta temporal de permisos;
- límites o fallos del servicio de Google.

Solo una ausencia confirmada se trata como eliminación real.

Si desaparece únicamente un adjunto, se elimina ese recurso sin eliminar la nota completa.

Esta lógica evita que un problema temporal con Google Drive provoque pérdida de información almacenada en la aplicación.

---

## 7. Integración de vistas del frontend

Las tareas `2_4_17` y `2_4_18` estuvieron enfocadas principalmente en conectar vistas existentes con servicios reales del backend.

En estas tareas no se atribuyen como propios los backends originalmente desarrollados por otros integrantes.

Mi aporte se concentró en integración frontend, navegación, estados de carga y error, y correcciones detectadas durante las pruebas.

### 7.1 Grupos

Se conectó la vista de grupos con los contratos reales de Social.

El trabajo incluyó:

- carga de grupos;
- información del grupo;
- miembros;
- actualización de la interfaz;
- ajustes en el flujo de salida y transferencia de administración.

### 7.2 Todas las notas

Se integró la vista principal de Notes con el backend real.

Entre las áreas trabajadas se encuentran:

- creación y carga de notas;
- edición;
- sincronización con Google Drive;
- adjuntos;
- eliminación individual de adjuntos;
- estados de sincronización;
- reconciliación;
- manejo de archivos eliminados directamente desde Drive;
- mensajes de validación.

La implementación original del backend de Notes corresponde principalmente a Héctor. Mi trabajo en esta sección se concentró en integración y fixes posteriores.

### 7.3 Perfil y barra lateral

Se conectó la vista de perfil con `GET /profile/me` y `PATCH /profile/me`.

También se integró la barra lateral global como punto de navegación entre:

- grupos;
- notas;
- perfil;
- funcionalidades asociadas a los grupos.

### 7.4 Sprint

La vista de Sprint fue conectada con los endpoints existentes del backend correspondiente.

Se trabajó con:

- hojas de Sprint;
- tareas;
- estados;
- fechas;
- registro y visualización de horas;
- actualización de la vista después de modificaciones.

El backend principal de Sprint corresponde a Martín; mi aporte fue principalmente la integración frontend y sus correcciones.

### 7.5 Kanban

La vista Kanban se conectó con los endpoints de tareas del grupo.

El frontend utiliza datos reales del backend para representar estados y cambios de las tareas.

El backend principal de Kanban corresponde a Martín.

### 7.6 Reuniones y Google Calendar

Se conectó la vista para agendar reuniones con los servicios existentes.

La integración contempla:

- datos básicos de la reunión;
- participantes;
- invitados;
- estados de carga;
- errores;
- integración con Google Calendar cuando corresponde.

El backend principal de reuniones pertenece al módulo Social desarrollado por otros integrantes; mi trabajo fue la conexión de la interfaz y las correcciones de integración.

### 7.7 Chat grupal

La vista de chat se conectó con Stream.

La integración permite:

- obtener el token de Stream;
- asociar el canal con el grupo;
- abrir la conversación;
- cargar mensajes;
- manejar estados de conexión y errores.

La infraestructura de Stream fue desarrollada en el backend por otro integrante. Mi aporte se concentró en integrar la vista con esos servicios.

---

## 8. Manejo de estados y seguridad en frontend

Durante la integración se trabajó para mantener estados coherentes como:

- cargando;
- sincronizando;
- completado;
- sin conexión;
- error de backend;
- error de servicio externo;
- datos no disponibles.

La interfaz evita mostrar al usuario información interna que no necesita conocer, como:

- tokens;
- credenciales;
- errores SQL;
- endpoints internos;
- identificadores técnicos innecesarios;
- información privada de servicios externos.

La responsabilidad final sobre autenticación, autorización y validación de permisos permanece en el backend.

---

## 9. Documentación de endpoints y lógica de negocio

En la tarea `2_4_19` documenté únicamente los endpoints y aportes que implementé directamente o en los que realicé una contribución sustancial.

Entre los endpoints propios documentados se encuentran:

- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /profile/me`
- `PATCH /profile/me`
- `POST /auth/google-drive/connect`
- `GET /auth/google-drive/status`
- `POST /notes/reconcile`

También documenté mi aporte colaborativo en:

- `GET /notes/:id/access`

En esta documentación se evitó atribuir como propios endpoints base de Groups, Sprint, Kanban, Meetings, Chat, Attachments o Notes cuando su implementación original corresponde a otros integrantes.

---

## 10. Revisión de integridad del código de Martín

La tarea `2_4_20` consistió en revisar el código asociado principalmente a:

- Kanban;
- Sprint;
- registro de horas;
- reuniones;
- Google Calendar;
- Discord;
- Stream.

La revisión final concluyó con el resultado:

**APROBADO CON OBSERVACIONES.**

En el estado revisado no se encontraron problemas críticos de seguridad.

Se verificó que los problemas anteriores relacionados con membresía, acceso entre grupos y asignación de tareas habían sido corregidos.

Las observaciones restantes se relacionan principalmente con diferencias entre contratos documentados y comportamiento real, además de algunos casos menores de resiliencia en integraciones externas.

---

## 11. Reglas de negocio

En la tarea `2_4_22` se consolidaron y actualizaron las reglas de negocio de los módulos principales.

La documentación quedó organizada en:

- Identity;
- Academic;
- Notes;
- Social;
- Asistente de IA;
- reglas transversales;
- funcionalidades fuera de alcance.

Entre los puntos importantes se dejó explícito que:

- Identity no utiliza un rol global;
- los permisos administrativos dependen del rol dentro de cada grupo;
- las sesiones pueden coexistir y se revocan individualmente;
- la identidad de una operación protegida debe provenir de la sesión;
- Notes utiliza Google Drive como almacenamiento externo;
- los errores temporales de Google no deben causar pérdida automática de datos;
- la creación de reuniones es independiente de Calendar, Discord y Stream;
- el asistente de IA todavía no se encuentra implementado como funcionalidad disponible;
- algunas reglas académicas continúan pendientes de implementación completa.

---

## 12. Diagrama de casos de uso

La tarea `2_4_23` consistió en actualizar el diagrama de casos de uso para representar el estado actual del sistema.

El diagrama se ajustó para representar principalmente los roles y módulos que continúan formando parte de la aplicación.

Entre los actores principales se consideran:

- Usuario / estudiante.
- Administrador de grupo.

También se representan como sistemas externos las integraciones correspondientes, como:

- Google Drive;
- Google Calendar;
- Stream;
- Discord.

El diagrama fue simplificado para mostrar las funciones principales del sistema sin convertir procesos internos, como renovación de tokens o llamadas técnicas a APIs, en acciones directas del usuario.

---

## 13. Investigación de documentación de otros integrantes

La tarea `2_4_21` corresponde a revisar la documentación de:

- Miguel;
- Héctor;
- Benjamín.

Esta tarea se encuentra en curso y se documenta de forma separada mediante un resumen general de las áreas cubiertas, puntos claros, vacíos y aspectos que requieren actualización o verificación.

No se incluye todavía una conclusión definitiva mientras falten integrantes por revisar.

---

## 14. Tests asociados a mi trabajo

Las áreas principales cuentan con pruebas relacionadas con:

### Auth

- refresh válido;
- refresh revocado;
- refresh expirado;
- refresh inexistente;
- rotación;
- reuso de token anterior;
- logout activo;
- logout idempotente.

### Perfil

- lectura;
- actualización parcial;
- visibilidad válida e inválida;
- límites de campos;
- aislamiento por usuario.

### Google Drive

- conexión;
- token vigente;
- token expirado;
- renovación;
- revocación;
- estado de reconexión;
- desconexión;
- logs sin tokens.

### Notes / Drive

- acceso;
- compartición restricted;
- permisos nominales;
- reconciliación;
- diferencia entre archivo eliminado y fallo temporal.

Las pruebas de integraciones externas utilizan valores de prueba, mocks o stubs cuando corresponde y no requieren publicar secretos reales.

---

## 15. Separación de responsabilidades

Una parte importante de la documentación consiste en diferenciar claramente implementación propia de trabajo colaborativo.

### Implementación directa

Entre mis principales implementaciones directas se encuentran:

- refresh de sesión;
- rotación del refresh token;
- logout;
- perfil;
- visibilidad del perfil;
- conexión Google Drive;
- renovación automática de Google Drive;
- detección de revocación;
- consulta del estado de Drive;
- permisos nominales restricted;
- reconciliación Notes ↔ Drive;
- integración de las vistas asignadas.

### Trabajo colaborativo

Algunos módulos tenían una implementación base creada por otros integrantes.

En estos casos mi aporte fue principalmente integración, endurecimiento o corrección.

Ejemplos:

- `GET /notes/:id/access`: base del módulo Notes y aporte propio sobre permisos y acceso.
- Notes: backend original principalmente de Héctor; integración frontend y fixes posteriores de mi parte.
- Sprint y Kanban: backend principal de Martín; integración frontend de mi parte.
- Meetings, Stream y Social: backend desarrollado por otros integrantes; conexión de vistas e integración desde frontend.

Esta separación evita atribuir como propia una implementación original ajena y permite identificar con mayor claridad mi aporte real dentro del proyecto.

---

## 16. Archivos y áreas principales relacionadas

El trabajo se distribuye principalmente entre:

- `back/auth/`
- `back/notes/`
- `front/lib/`
- documentación del proyecto;
- contratos y reglas de negocio;
- diagramas y material de cierre del Sprint.

Dentro de Auth se concentra la lógica de sesión, perfil y OAuth.

Dentro de Notes se encuentran los aportes relacionados con acceso, permisos restricted y reconciliación.

En Flutter se concentra la integración de las vistas con los servicios reales.

---

## 17. Conclusión

Durante el Sprint 1 mi trabajo se concentró en completar funciones necesarias para conectar autenticación, perfil, Google Drive, Notes y las principales vistas de la aplicación.

En Auth implementé la renovación y cierre de sesiones, el perfil y la integración OAuth con Google Drive. También agregué el manejo de expiración y revocación de las credenciales externas.

En Notes participé en funcionalidades colaborativas relacionadas con control de acceso, permisos restringidos y reconciliación con Google Drive, manteniendo la separación entre mi aporte y la implementación original del módulo.

En frontend conecté distintas vistas con los servicios reales del sistema, incluyendo grupos, Notes, perfil, Sprint, Kanban, reuniones y chat.

Finalmente, participé en tareas de cierre y revisión del Sprint, como documentación de endpoints, revisión de integridad de código, actualización de reglas de negocio y actualización del diagrama de casos de uso.

La única tarea que continúa en desarrollo dentro de este resumen es la investigación de documentación de Miguel, Héctor y Benjamín, que se completará de forma separada.
