-- Rubrik lama yang slug-nya sudah tidak dipakai tetap dikenali:
-- /bisnis, /tren, dst. dialihkan (301) ke rubrik penggantinya, dan engine/Studio
-- yang masih mengirim kategori lama tetap masuk ke rubrik yang benar.
CREATE TABLE IF NOT EXISTS category_aliases (
    site_id     BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    slug        TEXT NOT NULL,
    category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (site_id, slug)
);

-- Ekonomi + Bisnis -> Ekonomi & Bisnis (/ekonomi-bisnis); Tren -> Viral.
DO $$
DECLARE
    st RECORD; eko BIGINT; bis BIGINT; keep BIGINT; vir BIGINT; tre BIGINT;
BEGIN
    FOR st IN SELECT id FROM sites LOOP
        eko := NULL; bis := NULL; vir := NULL; tre := NULL;
        SELECT id INTO eko FROM categories WHERE site_id = st.id AND slug = 'ekonomi';
        SELECT id INTO bis FROM categories WHERE site_id = st.id AND slug = 'bisnis';
        keep := COALESCE(eko, bis);
        IF keep IS NOT NULL AND NOT EXISTS (SELECT 1 FROM categories WHERE site_id = st.id AND slug = 'ekonomi-bisnis') THEN
            IF eko IS NOT NULL AND bis IS NOT NULL THEN
                UPDATE articles SET category_id = keep WHERE category_id = bis;
                DELETE FROM categories WHERE id = bis;
            END IF;
            UPDATE categories SET slug = 'ekonomi-bisnis', name = 'Ekonomi & Bisnis' WHERE id = keep;
            INSERT INTO category_aliases (site_id, slug, category_id) VALUES (st.id, 'ekonomi', keep), (st.id, 'bisnis', keep)
                ON CONFLICT (site_id, slug) DO UPDATE SET category_id = EXCLUDED.category_id;
        END IF;

        SELECT id INTO vir FROM categories WHERE site_id = st.id AND slug = 'viral';
        SELECT id INTO tre FROM categories WHERE site_id = st.id AND slug = 'tren';
        IF vir IS NOT NULL AND tre IS NOT NULL THEN
            UPDATE articles SET category_id = vir WHERE category_id = tre;
            DELETE FROM categories WHERE id = tre;
            INSERT INTO category_aliases (site_id, slug, category_id) VALUES (st.id, 'tren', vir)
                ON CONFLICT (site_id, slug) DO UPDATE SET category_id = EXCLUDED.category_id;
        END IF;
    END LOOP;
END $$;
