# Recuperación de contraseña

Desde «¿Olvidaste tu clave?» se ingresa el correo, se recibe un código de ocho
dígitos y se establece la contraseña nueva. El remitente predeterminado es
`Sigma Academy <sigmaacademy.noreply@gmail.com>`.

Web y aplicaciones instaladas usan Auth en
`https://ti3-brojas.dev.censei.cl/api-auth`. Auth se conecta al servicio
`postgres:5432` dentro de `student-brojas` en Pillán. Supabase no interviene.
Las rutas relativas `/api-*` del build web también resuelven a Pillán en nativo.

## Activación

1. En la cuenta de Google indicada, habilitar la verificación en dos pasos y
   generar una [contraseña de aplicación](https://support.google.com/accounts/answer/185833).
2. Copiar las variables de `back/auth/.env.example` al `.env` privado de la raíz
   y completar `SMTP_PASSWORD` con la contraseña de aplicación sin espacios.
   No subir la credencial a Git. El cliente Flutter no recibe este secreto.
   Se usa [SMTP Gmail con STARTTLS en el puerto 587](https://support.google.com/mail/answer/7104828).
3. Con `KUBECONFIG` apuntando al acceso de Pillán, ejecutar desde PowerShell:
   `./scripts/configure-pillan-recovery.ps1`.
   Lee solamente SMTP_PASSWORD del `.env`, agrega las cuatro variables SMTP a
   `ti3-secrets` sin reemplazar sus otras claves y aplica la migración al
   PostgreSQL de Pillán. Comprueba primero que DATABASE_URL apunte a `postgres`.
   Se puede indicar `-Kubectl <ruta>` si kubectl no está en PATH.
4. Publicar los cambios en Development y esperar a que `build-images` termine.
   Actualizar las imágenes de los deployments `auth` y `front` al SHA publicado
   con `kubectl -n student-brojas set image deployment/auth auth=ghcr.io/kurovoxx/tallerintegracion3-auth:<SHA>`
   y el comando equivalente para `front`. Esperar `kubectl rollout status` de ambos.
   Si solamente cambió SMTP, usar `-RestartAuth` en el script del paso 3.
   Sin SMTP_PASSWORD, la recuperación devuelve 503; el resto de Auth funciona.

La migración `back/auth/db/migrations/001_password_resets.sql` es aditiva e
idempotente. Las bases nuevas la incluyen en `docker/postgres/init.sql`;
reiniciar un volumen existente no vuelve a ejecutar ese archivo.

## Aplicaciones instaladas y desarrollo

- `flutter run -d windows` / `flutter build windows --release` usan Pillán por
  defecto, igual que Android/Linux. No incluyen contraseñas SMTP ni de PostgreSQL.
- Docker web usa nginx y `/api-*` en el mismo origen. Los builds nativos usan
  las URLs públicas completas; no requieren un servidor local.
- Para ejecutar un backend local contra la misma base, abrir un túnel:
  `kubectl -n student-brojas port-forward service/postgres 15432:5432`.
  El `.env` privado utiliza `127.0.0.1:15432`; las credenciales se obtienen del
  Secret de Pillán y nunca deben copiarse a archivos versionados o al frontend.
- Para backends totalmente locales, pasar explícitamente los valores de
  `front/.env.local.example` mediante `--dart-define`.
- Pillán sirve certificados de desarrollo diferentes por réplica del Ingress.
  Desktop/móvil verifican su SHA-256, vigencia, hostname y puerto 443. Una
  rotación requiere actualizar las huellas verificadas (override
  `PILLAN_CERT_SHA256`, lista separada por comas) y recompilar. Ya no se acepta
  cualquier certificado con `ALLOW_INSECURE`. TLS válido sigue funcionando
  mediante la validación normal del sistema.
- El navegador administra su propia confianza: el aviso del certificado de
  desarrollo persistirá hasta que los administradores de Pillán instalen un
  certificado público válido. El frontend no puede eliminar ese aviso.

## Contrato

- `POST /auth/forgot-password`: `{"email":"usuario@ejemplo.com"}` → 200 con
  mensaje genérico tanto para correos registrados como desconocidos y reenvíos
  dentro del período de espera. Si falla SMTP, se registra un aviso sin datos
  personales y se conserva la respuesta genérica; el usuario puede reintentar
  luego de 60 segundos. La respuesta no confirma entrega de correo.
- `POST /auth/reset-password`:
  `{"email":"usuario@ejemplo.com","code":"12345678","password":"NuevaClave9"}`
  → 200 cuando se actualiza la contraseña; 400 si el código no es válido, expiró,
  se usó o agotó sus intentos, o si la contraseña no cumple la política vigente.

## Protección y verificación

- Código aleatorio, almacenado con bcrypt, expira a los 15 minutos; cinco
  intentos por código. Un nuevo envío reemplaza el código anterior y se permite
  como máximo una vez cada 60 segundos por cuenta, compartido entre réplicas.
- Cambio de contraseña, consumo de código y revocación de refresh tokens en una
  transacción. Los access tokens ya emitidos siguen válidos hasta su expiración
  habitual (15 minutos por defecto).
- La contraseña conserva la política actual del backend: 8–72 bytes, una letra,
  un dígito, sin espacios. El código y la contraseña nunca se escriben en logs.
- Pruebas backend: `cd back/auth` y `go test ./...`.
- Pruebas de repositorio: definir `PASSWORD_RESET_TEST_DATABASE_URL` apuntando
  exclusivamente a una base PostgreSQL de pruebas vacía; ejecutar
  `go test ./internal/repository -run PasswordReset`.
- Pruebas Flutter: `cd front` y `flutter test test/password_recovery_test.dart`.
- Prueba manual con SMTP configurado: solicitar un código desde una cuenta
  propia de prueba, recibirlo, cambiar contraseña, entrar con la nueva y
  comprobar que la anterior y el código usado dejan de funcionar.

Rollback: revertir la aplicación; la tabla nueva puede conservarse sin afectar
la versión anterior. No se elimina información existente con esta migración.
