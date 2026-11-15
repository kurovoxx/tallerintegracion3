-- Miniatura de la foto de perfil para listados de grupos. Se genera en el
-- cliente a partir del mismo recorte y evita enviar la foto completa (data URL)
-- en cada miembro. text por el mismo motivo que photo_url.
ALTER TABLE identity.profiles
  ADD COLUMN IF NOT EXISTS photo_thumb_url text;
