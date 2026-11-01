# Refresh tokens: diseño, despliegue y validación

## Diseño

Las sesiones nuevas guardan el bcrypt existente en `token_hash` y una huella
SHA-256 del refresh en `token_fingerprint`. El lookup por huella usa el índice
único parcial y también encuentra filas revocadas o vencidas para que el
servicio las rechace. La rotación permanece transaccional, bloquea y revalida
el token anterior, revoca ese token e inserta el nuevo par de hashes antes del
commit. Dos renovaciones concurrentes del mismo refresh solo pueden completar
una rotación.

Las sesiones anteriores no se pueden convertir directamente: la base solo
conserva bcrypt, no el refresh crudo. Si no hay coincidencia por huella, el
fallback selecciona filas legacy con `token_fingerprint IS NULL`, vigentes y
no revocadas; materializa los hashes y libera la conexión antes de comparar
bcrypt. Este camino conserva compatibilidad, pero todavía puede ser lento si
quedan muchos candidatos válidos. Cada sesión legacy pasa al lookup indexado
cuando su usuario renueva.

## Migraciones y despliegue

La migración `db/migrations/002_refresh_token_fingerprint.sql` añade la
columna nullable. `003_refresh_token_fingerprint_index.sql` crea el índice
parcial mediante `CREATE INDEX CONCURRENTLY`. `docker/postgres/init.sql`
incluye ambos para bases nuevas. Aplicar primero las migraciones y verificar
columna e índice; desplegar el binario nuevo después. `CREATE INDEX
CONCURRENTLY` debe ejecutarse fuera de una transacción.

El mecanismo operativo usado por el proyecto envía el SQL por stdin a `psql`
mediante `kubectl exec -i`, como en `scripts/configure-pillan-recovery.ps1`.
Desde la raíz del repositorio, usar invocaciones separadas:

```powershell
Get-Content -Raw .\back\auth\db\migrations\002_refresh_token_fingerprint.sql |
  kubectl -n student-brojas exec -i deployment/postgres -- psql -U postgres -d postgres -v ON_ERROR_STOP=1
Get-Content -Raw .\back\auth\db\migrations\003_refresh_token_fingerprint_index.sql |
  kubectl -n student-brojas exec -i deployment/postgres -- psql -U postgres -d postgres -v ON_ERROR_STOP=1
```

Antes de desplegar, comprobar que existe la columna y que el índice está listo
y válido:

```sql
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'identity'
  AND table_name = 'refresh_tokens'
  AND column_name = 'token_fingerprint';

SELECT c.relname, i.indisvalid, i.indisready, pg_get_indexdef(i.indexrelid)
FROM pg_index AS i
JOIN pg_class AS c ON c.oid = i.indexrelid
WHERE c.relname = 'idx_refresh_tokens_fingerprint';
```

El índice debe mostrar `indisvalid = true` e `indisready = true`. Si la creación
concurrente falla y deja un índice inválido, no desplegar el binario nuevo;
revisar el índice inválido, retirarlo según el procedimiento DBA y repetir la
migración. Tras verificar el esquema, desplegar Auth y esperar el rollout:

```powershell
kubectl -n student-brojas rollout status deployment/auth
```

Comprobar `/health` y vigilar resultados y duración de refresh sin registrar
tokens, cuerpos HTTP, correos ni datos personales.

## Reversión y retirada del fallback

La reversión normal es volver al ReplicaSet anterior y conservar la columna y
el índice: el esquema es aditivo, el binario anterior sigue leyendo
`token_hash`, y el nuevo acepta filas creadas por el binario anterior mediante
el fallback.

```powershell
kubectl -n student-brojas rollout undo deployment/auth
kubectl -n student-brojas rollout status deployment/auth
```

Solo si se decidió retirar el esquema y ya no queda ningún pod nuevo, ejecutar
el borrado del índice como sentencia independiente y después quitar la
columna. No es necesario para una reversión normal:

```powershell
kubectl -n student-brojas exec -i deployment/postgres -- psql -U postgres -d postgres -v ON_ERROR_STOP=1 -c "DROP INDEX CONCURRENTLY IF EXISTS identity.idx_refresh_tokens_fingerprint"
kubectl -n student-brojas exec -i deployment/postgres -- psql -U postgres -d postgres -v ON_ERROR_STOP=1 -c "ALTER TABLE identity.refresh_tokens DROP COLUMN IF EXISTS token_fingerprint"
```

No retirar el fallback por una espera fija de siete días. Considerar el TTL
máximo configurado y el tiempo desde que dejaron de existir escritores
antiguos, con margen operativo. Además, comprobar en la base objetivo que no
queden sesiones legacy vigentes:

```sql
SELECT count(*) AS active_legacy_sessions
FROM identity.refresh_tokens
WHERE token_fingerprint IS NULL
  AND revoked = false
  AND expires_at > now();
```

Mantener el fallback mientras el conteo sea mayor que cero.

## Validación y límites

**Validado localmente:** cinco pruebas de integración con PostgreSQL 16
desechable aprobaron, ninguna quedó omitida. Cubrieron flujo register/login/
refresh/logout, migración desde un esquema anterior, tokens legacy y
vencimiento, rechazo de reutilización, dos refresh concurrentes y uso del
índice para el lookup nuevo con 3000 filas legacy. También se inicializó una
segunda base vacía con `docker/postgres/init.sql` y se comprobó la columna y el
índice.

Las regresiones frontend sumaron 26 aprobadas. Un flujo integrado local con
Auth y PostgreSQL reales completó registro, login, un refresh, persistencia de
la pareja rotada, restauración en un segundo proceso y `GET /profile/me` con
HTTP 200. El harness QA usado en esa ejecución es auxiliar y se conserva fuera
del cambio principal; estos resultados locales no validan Pillán.

**Pendiente en Pillán:** la ejecución remota informó login HTTP 200 en 2383 ms
y un refresh que agotó 10001 ms sin status HTTP. La prueba se detuvo antes de
verificar persistencia, reinicio o petición protegida. No se ha ejecutado la
migración contra Pillán ni se ha completado allí una validación funcional.
Un diagnóstico sintético in-memory superó 10 s con 192 comparaciones bcrypt
en esta máquina; no midió SQL ni confirma que bcrypt, el pool, locks, ingress,
servidor o red causaran el timeout remoto. Hace falta una nueva medición
sanitizada en Pillán que complete refresh, guardado, restauración y petición
protegida para validar el resultado.
