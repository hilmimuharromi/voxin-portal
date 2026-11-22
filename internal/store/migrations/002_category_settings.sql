-- Rubrik bisa diatur dari CMS: deskripsi (meta description halaman rubrik) dan
-- pilihan tampil sebagai blok di beranda.
ALTER TABLE categories ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE categories ADD COLUMN IF NOT EXISTS on_home BOOLEAN NOT NULL DEFAULT true;
