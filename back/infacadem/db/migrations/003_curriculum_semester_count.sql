-- Cantidad de semestres de la malla (1..20); 8 por defecto.
ALTER TABLE academic.curricula
    ADD COLUMN IF NOT EXISTS semester_count int NOT NULL DEFAULT 8
    CHECK (semester_count BETWEEN 1 AND 20);
