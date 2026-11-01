# Reglas de negocio — versión final con priorización MoSCoW

## 0. Criterio de priorización

Las reglas se clasifican según MoSCoW para indicar su prioridad dentro del estado actual del proyecto:

- **MUST:** necesaria para el funcionamiento principal, la seguridad o la consistencia del sistema.
- **SHOULD:** importante y deseable, pero el sistema puede seguir funcionando sin ella.
- **COULD:** mejora o funcionalidad secundaria que aporta valor, pero no es indispensable para el funcionamiento principal.
- **WON'T:** fuera del alcance de la entrega actual. Puede considerarse para trabajo futuro.

La prioridad se aplica al estado actual del proyecto y no significa que una funcionalidad marcada como **WON'T** quede descartada permanentemente.

---

## 1. Identity y sesiones

### MUST

**RN-IDENT-01.** El email de cada usuario debe tener un formato válido y ser único. Antes de persistirse debe normalizarse para evitar duplicados equivalentes.

**RN-IDENT-02.** Las contraseñas deben almacenarse utilizando hash seguro y nunca deben guardarse ni devolverse en texto plano. Los refresh tokens almacenados también deben mantenerse mediante hash.

**RN-IDENT-03.** La identidad utilizada para autorizar una operación protegida debe obtenerse desde la sesión autenticada y no desde un `user_id` enviado libremente por el cliente.

**RN-IDENT-04.** Un usuario puede mantener varias sesiones activas. La renovación o cierre de una sesión no debe invalidar automáticamente las demás.

**RN-IDENT-05.** Cuando un refresh token se utiliza correctamente debe rotarse: el token anterior queda revocado y se entrega uno nuevo.

**RN-IDENT-06.** Un refresh token inexistente, expirado o revocado no puede utilizarse para renovar una sesión.

**RN-IDENT-07.** El logout revoca únicamente la sesión correspondiente al refresh token utilizado y debe ser idempotente.

**RN-IDENT-08.** Identity no utiliza un rol global para determinar permisos de grupos. Los permisos de grupo dependen de la membresía y del rol del usuario dentro de cada grupo.

### SHOULD

**RN-IDENT-09.** La visibilidad del perfil admite los valores `public` y `private`. La configuración debe mantenerse persistida aunque el flujo completo de consulta pública de perfiles no se encuentre implementado todavía.

---

## 2. Perfil de usuario

### MUST

**RN-PERFIL-01.** Un usuario autenticado solo puede consultar y modificar su propio perfil mediante los endpoints destinados a la sesión actual.

**RN-PERFIL-02.** La actualización del perfil es parcial: los campos que no son enviados deben conservar su valor anterior.

**RN-PERFIL-03.** Un request inválido debe rechazarse antes de persistir cambios parciales no deseados.

### SHOULD

**RN-PERFIL-04.** La interfaz debe informar de forma comprensible los estados de carga, validación, sesión no válida, perfil inexistente y error interno.

---

## 3. Google Drive

### MUST

**RN-DRIVE-01.** Las credenciales privadas de Google deben permanecer en el backend. Los access tokens y refresh tokens de Google no deben exponerse al frontend ni registrarse en logs.

**RN-DRIVE-02.** La conexión con Google Drive debe quedar asociada al usuario autenticado.

**RN-DRIVE-03.** Cuando el access token de Google está vencido o próximo a expirar, el sistema debe intentar renovarlo utilizando el refresh token correspondiente.

**RN-DRIVE-04.** Cuando una conexión ya no pueda renovarse de forma permanente, el sistema debe indicar que el usuario necesita reconectar Google Drive.

**RN-DRIVE-05.** Los errores temporales de Google, como problemas de red, límites de servicio o errores del servidor, no deben marcar permanentemente una conexión válida como revocada.

**RN-DRIVE-06.** Al reconectar una cuenta, un refresh token anterior solo puede conservarse cuando corresponda a la misma cuenta de Google. No debe reutilizarse una credencial anterior perteneciente a otra cuenta.

### SHOULD

**RN-DRIVE-07.** La interfaz debe mostrar al usuario el estado de conexión de Drive y permitir reconocer claramente cuándo se requiere una reconexión.

---

## 4. Notes

### MUST

**RN-NOTES-01.** Un apunte solo puede ser modificado o eliminado por su autor.

**RN-NOTES-02.** El título de un apunte es obligatorio.

**RN-NOTES-03.** La visibilidad individual de una nota puede ser `private` o `public`.

**RN-NOTES-04.** Una nota privada solo puede ser consultada por su autor o mediante un mecanismo explícito de acceso permitido por el sistema.

**RN-NOTES-05.** Una nota pública puede ser consultada de acuerdo con las reglas de visibilidad sin exigir seguimiento previo de su autor.

**RN-NOTES-06.** Cuando un recurso no existe o un usuario no está autorizado para consultarlo, la API debe evitar revelar información innecesaria sobre su existencia.

**RN-NOTES-07.** Las notas sincronizadas utilizan Google Drive como almacenamiento externo del contenido. La aplicación mantiene la información necesaria para relacionar la metadata de Notes con sus archivos externos.

**RN-NOTES-08.** Los adjuntos admitidos por el sistema deben respetar los tipos y límites configurados. Actualmente se consideran imágenes y documentos PDF.

**RN-NOTES-09.** Una eliminación debe contemplar la metadata de la aplicación y los archivos externos relacionados.

**RN-NOTES-10.** Un fallo temporal de Google Drive no debe provocar la eliminación automática de una nota o adjunto.

**RN-NOTES-11.** Cuando una operación externa no pueda completarse inmediatamente, el sistema debe conservar información suficiente para permitir reintento o reconciliación.

**RN-NOTES-12.** Si un archivo es eliminado directamente desde Google Drive, la aplicación solo debe considerarlo eliminado cuando la ausencia pueda confirmarse de forma inequívoca, por ejemplo porque el archivo no existe o se encuentra en la papelera.

**RN-NOTES-13.** Las actualizaciones de una nota deben protegerse frente a modificaciones antiguas que intenten sobrescribir silenciosamente una versión más reciente.

### SHOULD

**RN-NOTES-14.** Un usuario puede guardar una referencia a una nota accesible de otro usuario. El acceso debe volver a comprobarse cuando la referencia sea utilizada.

**RN-NOTES-15.** Si la nota original de una referencia guardada deja de existir, la referencia también deja de estar disponible.

**RN-NOTES-16.** Cada usuario puede registrar como máximo un "me gusta" sobre una misma nota.

**RN-NOTES-17.** Una nota recién creada puede permanecer temporalmente en estado de sincronización mientras se completa su creación remota. La interfaz debe representar ese estado de forma comprensible.

### COULD

**RN-NOTES-18.** Un usuario puede copiar una nota accesible a su propia colección. La copia es independiente del original y puede modificarse posteriormente sin afectarlo.

---

## 5. Compartición de Notes y permisos restricted

### MUST

**RN-SHARE-01.** Compartir una nota con un grupo constituye un mecanismo de acceso independiente de la visibilidad individual de la nota.

**RN-SHARE-02.** Las notas compartidas pueden utilizar los modos `link` o `restricted`.

**RN-SHARE-03.** El modo `restricted` debe limitar el acceso a los miembros autorizados y mantener permisos nominales de lectura cuando corresponde.

**RN-SHARE-04.** Los cambios de membresía de un grupo deben provocar una nueva sincronización de los permisos `restricted` relacionados con las notas compartidas de ese grupo.

**RN-SHARE-05.** Cuando un usuario entra a un grupo, debe adquirir los permisos requeridos por las notas `restricted` a las que obtiene acceso.

**RN-SHARE-06.** Cuando un usuario abandona un grupo, es expulsado o baneado, debe perder los permisos que ya no necesita.

**RN-SHARE-07.** Si una misma nota está compartida con varios grupos y un usuario continúa autorizado mediante otro de ellos, su permiso no debe revocarse.

**RN-SHARE-08.** Un fallo temporal al modificar permisos de Drive debe conservar el estado pendiente o fallido para permitir un reintento posterior. No debe eliminar la nota.

**RN-SHARE-09.** Las rutas internas utilizadas entre Notes y Social para resolver membresía y correos deben ser de uso servicio-a-servicio y no deben exponerse como endpoints públicos.

**RN-SHARE-10.** Un error del servicio Social no debe interpretarse silenciosamente como un grupo sin miembros.

**RN-SHARE-11.** Los listados internos de correos utilizados para permisos de Drive no deben exponerse públicamente.

### SHOULD

**RN-SHARE-12.** Un administrador de grupo puede retirar una nota compartida del grupo sin eliminar la nota original de su autor.

**RN-SHARE-13.** El estado de sincronización de permisos debe poder distinguir entre operaciones sincronizadas, pendientes y fallidas para facilitar su reconciliación.

---

## 6. Grupos y permisos

### MUST

**RN-GRUPO-01.** El creador de un grupo queda registrado inicialmente como administrador.

**RN-GRUPO-02.** Dentro de un grupo existen los roles `admin` y `member`.

**RN-GRUPO-03.** Las operaciones administrativas, como gestionar miembros o regenerar invitaciones, requieren permisos de administrador.

**RN-GRUPO-04.** El acceso mediante invitación requiere un token válido.

**RN-GRUPO-05.** Cuando se regenera un token de invitación, los enlaces creados con el token anterior dejan de ser válidos.

**RN-GRUPO-06.** Un usuario baneado no puede volver a incorporarse mediante una invitación válida.

**RN-GRUPO-07.** Una expulsión y un baneo tienen comportamientos diferentes: el baneo mantiene una prohibición de reingreso y la expulsión simple no.

**RN-GRUPO-08.** Un administrador no puede expulsar ni banear a otro administrador mediante las operaciones normales de gestión.

**RN-GRUPO-09.** La administración puede transferirse a otro miembro del mismo grupo.

**RN-GRUPO-10.** Si el único administrador abandona el grupo, debe promoverse a otro miembro de acuerdo con las reglas definidas. Si no queda ningún miembro, el grupo puede eliminarse.

**RN-GRUPO-11.** El usuario asignado a una tarea debe pertenecer al grupo correspondiente.

---

## 7. Sprint y Kanban

### MUST

**RN-PROD-01.** Las hojas de Sprint y las tareas deben pertenecer al grupo correspondiente.

**RN-PROD-02.** Las operaciones de Sprint y Kanban deben verificar que el usuario tenga acceso al grupo antes de operar sobre sus recursos.

**RN-PROD-03.** En las tareas de Sprint existe un usuario asignado obligatorio. En la lista To-Do/Kanban la asignación puede ser opcional.

**RN-PROD-04.** Una tarea de otro grupo no puede ser modificada o eliminada utilizando únicamente su identificador.

**RN-PROD-05.** El registro de horas debe asociarse a una tarea válida y mantener la pertenencia al grupo correspondiente.

---

## 8. Reuniones, Calendar, Chat y Discord

### MUST

**RN-SOCIAL-01.** Actualmente cualquier miembro válido del grupo puede agendar una reunión.

**RN-SOCIAL-02.** La creación de una reunión dentro de la aplicación es la operación principal y no depende del éxito de servicios externos.

**RN-SOCIAL-03.** El chat grupal utiliza Stream y cada usuario debe acceder al canal correspondiente al grupo al que pertenece.

### SHOULD

**RN-SOCIAL-04.** Al crear una reunión, el sistema debe generar las notificaciones internas correspondientes, excluyendo al usuario que creó la reunión cuando así lo determine el flujo.

**RN-SOCIAL-05.** Google Calendar puede complementar el flujo de reuniones. Un fallo de Calendar no debe eliminar ni revertir una reunión ya creada.

**RN-SOCIAL-06.** Discord puede utilizarse como integración adicional del grupo para acceso, configuración o avisos relacionados con reuniones.

### COULD

**RN-SOCIAL-07.** La gestión completa del estado de lectura de las notificaciones internas puede incorporarse como una mejora posterior.

---

## 9. Academic

### MUST

**RN-ACADEMIC-01.** La simulación de notas debe ser una operación de consulta y no debe modificar las notas persistidas.

### COULD

Las siguientes capacidades forman parte del diseño académico, pero no constituyen reglas garantizadas de la entrega actual:

**RN-ACADEMIC-02.** Gestión completa de malla y materias.

**RN-ACADEMIC-03.** Registro completo de asistencia y ausencias desde un servicio backend.

**RN-ACADEMIC-04.** Validación de pesos de evaluaciones y subnotas.

**RN-ACADEMIC-05.** Gestión automática de examen final y redistribución de pesos.

**RN-ACADEMIC-06.** Aplicación automática de reglas de aprobación, reprobación y redondeo.

### WON'T

**RN-ACADEMIC-07.** La implementación completa de estas reglas académicas avanzadas no forma parte del alcance comprometido para esta entrega.

---

## 10. Asistente de IA

### WON'T

**RN-IA-01.** El asistente de inteligencia artificial no forma parte de la funcionalidad disponible en la entrega actual.

**RN-IA-02.** Las estructuras de base de datos preparadas para conversaciones o mensajes no deben presentarse como una funcionalidad disponible para el usuario.

**RN-IA-03.** El proveedor, contexto permitido y comportamiento definitivo del asistente deberán definirse antes de una implementación futura.

---

## 11. Reglas transversales

### MUST

**RN-TRANS-01.** Las operaciones protegidas requieren autenticación válida mediante los mecanismos definidos por cada servicio.

**RN-TRANS-02.** Registro, inicio de sesión y renovación de sesión pueden ejecutarse sin un access token previamente válido cuando el flujo correspondiente utiliza sus propias credenciales.

**RN-TRANS-03.** Las comunicaciones internas que permiten acceder a información sensible entre microservicios deben utilizar autenticación servicio-a-servicio y no exponerse como rutas públicas.

**RN-TRANS-04.** La verificación de propiedad de recursos y pertenencia a grupos debe realizarse en el backend y no quedar únicamente bajo responsabilidad del frontend.

**RN-TRANS-05.** Contraseñas, tokens, credenciales privadas y secretos de integraciones externas no deben exponerse en respuestas ni registrarse en logs.

**RN-TRANS-06.** Los fallos temporales de servicios externos no deben provocar pérdida automática de información persistida cuando exista un mecanismo de recuperación o reconciliación.

---

## 12. Resumen MoSCoW

| Prioridad | Alcance principal |
|---|---|
| **MUST** | Autenticación, sesiones, perfil propio, conexión segura con Drive, CRUD y acceso a Notes, permisos `restricted`, grupos, Sprint/Kanban, reuniones principales y chat. |
| **SHOULD** | Visibilidad completa de perfil, guardar/me gusta, estados visuales, Calendar, Discord, moderación y sincronización visible. |
| **COULD** | Copia de notas y mejoras académicas o de notificaciones que no son esenciales para el flujo principal. |
| **WON'T** | Asistente de IA y reglas académicas avanzadas que no forman parte de la entrega actual. |

---

## 13. Fuera de alcance de la entrega actual

Además de los elementos clasificados como **WON'T**, continúan fuera del alcance actual:

- un sistema completo de auditoría transaccional para todas las operaciones sensibles;
- un estado intermedio de archivado para grupos antes de su eliminación definitiva;
- un flujo completo de consulta pública de perfiles basado en su visibilidad;
- la implementación funcional completa del asistente de IA;
- la aplicación completa de reglas académicas avanzadas desde un servicio backend.

