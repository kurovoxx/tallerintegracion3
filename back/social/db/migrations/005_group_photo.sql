-- Foto del grupo: imagen completa recortada por el cliente y su miniatura.
-- Se guardan como data URLs o URLs http(s); NULL = sin foto.
ALTER TABLE social.groups
  ADD COLUMN IF NOT EXISTS photo_url text,
  ADD COLUMN IF NOT EXISTS photo_thumb_url text;
