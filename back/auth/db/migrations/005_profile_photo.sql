-- Foto de perfil editable desde la app. Se guarda como data URL de imagen
-- (JPEG/PNG ya recortada por el cliente) para no depender de almacenamiento
-- externo. text evita el límite de 500 caracteres pensado para URLs remotas.
ALTER TABLE identity.profiles
  ALTER COLUMN photo_url TYPE text;
