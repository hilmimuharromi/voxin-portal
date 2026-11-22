-- Titik fokus foto ("x% y%") untuk memotong foto di kartu/thumbnail tanpa memenggal wajah.
-- Kosong = otomatis (agak ke atas; foto potret lebih ke atas lagi).
ALTER TABLE media ADD COLUMN IF NOT EXISTS focus TEXT NOT NULL DEFAULT '';
