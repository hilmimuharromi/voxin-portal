-- Every content table carries site_id so a second site (e.g. terasberita.id)
-- can live in the same CMS later without a schema rewrite.

CREATE TABLE sites (
    id          BIGSERIAL PRIMARY KEY,
    key         TEXT NOT NULL UNIQUE,            -- voxin
    name        TEXT NOT NULL,                   -- VOXIN
    base_url    TEXT NOT NULL DEFAULT '',
    settings    JSONB NOT NULL DEFAULT '{}',     -- tagline, PT, alamat, sosmed, engine token...
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    slug          TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'wartawan',   -- admin | pemred | editor | wartawan | kontributor
    title         TEXT NOT NULL DEFAULT '',           -- jabatan
    region        TEXT NOT NULL DEFAULT '',           -- wilayah tugas
    ukw_level     TEXT NOT NULL DEFAULT '',           -- muda | madya | utama
    ukw_number    TEXT NOT NULL DEFAULT '',
    bio           TEXT NOT NULL DEFAULT '',
    photo         TEXT NOT NULL DEFAULT '',
    active        BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id        BIGSERIAL PRIMARY KEY,
    site_id   BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    slug      TEXT NOT NULL,
    name      TEXT NOT NULL,
    position  INT NOT NULL DEFAULT 0,
    in_nav    BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (site_id, slug)
);

CREATE TABLE regions (
    id          BIGSERIAL PRIMARY KEY,
    site_id     BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    slug        TEXT NOT NULL,
    name        TEXT NOT NULL,                  -- Kab. Kotawaringin Timur
    province    TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    logo        TEXT NOT NULL DEFAULT '',
    partner     BOOLEAN NOT NULL DEFAULT false, -- has an active partnership (shows logo)
    UNIQUE (site_id, slug)
);

CREATE TABLE media (
    id          BIGSERIAL PRIMARY KEY,
    site_id     BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,                  -- relative to data/media, original
    width       INT NOT NULL DEFAULT 0,
    height      INT NOT NULL DEFAULT 0,
    variants    JSONB NOT NULL DEFAULT '{}',    -- {"400": "path", "800": ..., "1200": ...}
    credit      TEXT NOT NULL DEFAULT '',
    caption     TEXT NOT NULL DEFAULT '',
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE articles (
    id              BIGSERIAL PRIMARY KEY,
    site_id         BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    type            TEXT NOT NULL DEFAULT 'berita',   -- berita | kurasi | kerja_sama | opini
    status          TEXT NOT NULL DEFAULT 'draft',    -- draft | review | scheduled | published | unpublished
    title           TEXT NOT NULL DEFAULT '',
    slug            TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    body_md         TEXT NOT NULL DEFAULT '',
    intinya         TEXT NOT NULL DEFAULT '',         -- up to 3 points, one per line
    highlight       TEXT NOT NULL DEFAULT '',         -- phrase in the title shown with stabilo
    category_id     BIGINT REFERENCES categories(id) ON DELETE SET NULL,
    region_id       BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    dateline        TEXT NOT NULL DEFAULT '',         -- JAKARTA
    author_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    editor_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    image_id        BIGINT REFERENCES media(id) ON DELETE SET NULL,
    source_name     TEXT NOT NULL DEFAULT '',         -- kurasi: "TechCrunch"
    source_url      TEXT NOT NULL DEFAULT '',
    ai_assisted     BOOLEAN NOT NULL DEFAULT false,
    engine_ref      TEXT NOT NULL DEFAULT '',         -- id from voxin-engine (dedup)
    unpublish_note  TEXT NOT NULL DEFAULT '',
    editor_note     TEXT NOT NULL DEFAULT '',
    published_at    TIMESTAMPTZ,
    scheduled_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX articles_site_slug ON articles(site_id, slug) WHERE slug <> '';
CREATE INDEX articles_public ON articles(site_id, status, published_at DESC);
CREATE INDEX articles_engine_ref ON articles(site_id, engine_ref) WHERE engine_ref <> '';

CREATE TABLE article_tags (
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    tag        TEXT NOT NULL,
    slug       TEXT NOT NULL,
    PRIMARY KEY (article_id, slug)
);
CREATE INDEX article_tags_slug ON article_tags(slug);

CREATE TABLE article_social (
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    platform   TEXT NOT NULL,                   -- instagram | tiktok | threads
    url        TEXT NOT NULL,
    PRIMARY KEY (article_id, platform)
);

-- old slugs keep working (301) after a title change
CREATE TABLE slug_redirects (
    site_id    BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    old_path   TEXT NOT NULL,
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    PRIMARY KEY (site_id, old_path)
);

CREATE TABLE article_versions (
    id          BIGSERIAL PRIMARY KEY,
    article_id  BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    body_md     TEXT NOT NULL,
    changed_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX article_versions_article ON article_versions(article_id, id DESC);

CREATE TABLE pages (
    site_id     BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    slug        TEXT NOT NULL,
    title       TEXT NOT NULL,
    body_md     TEXT NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, slug)
);

-- cookie-less analytics: one row per article per day per traffic source
CREATE TABLE pageviews_daily (
    site_id     BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    article_id  BIGINT NOT NULL DEFAULT 0,      -- 0 = non-article page
    day         DATE NOT NULL,
    source      TEXT NOT NULL,                  -- search | sosmed | langsung | lainnya
    views       INT NOT NULL DEFAULT 0,
    uniques     INT NOT NULL DEFAULT 0,
    PRIMARY KEY (site_id, article_id, day, source)
);
CREATE TABLE pageview_seen (
    day         DATE NOT NULL,
    article_id  BIGINT NOT NULL,
    visitor     TEXT NOT NULL,                  -- sha256(day + ip + ua + secret), not reversible
    PRIMARY KEY (day, article_id, visitor)
);

INSERT INTO sites (key, name, settings) VALUES ('voxin', 'VOXIN', '{"tagline":"Suarain yang penting.","about":"Berita penting & viral, langsung ke intinya. Vaksin anti-hoaks.","handle":"@voxin.media","instagram":"https://www.instagram.com/voxin.media","tiktok":"https://www.tiktok.com/@voxin.media","threads":"https://www.threads.net/@voxin.media","company":"[Nama PT Pers]","address":"[Alamat redaksi]","press_status":"[Status verifikasi Dewan Pers]","email":"halo@voxin.id"}');

INSERT INTO categories (site_id, slug, name, position) SELECT id, c.slug, c.name, c.pos FROM sites, (VALUES
    ('politik','Politik',1), ('ekonomi','Ekonomi',2), ('bisnis','Bisnis',3), ('viral','Viral',4), ('tren','Tren',5),
    ('teknologi','Teknologi',6), ('olahraga','Olahraga',7), ('cek-fakta','Cek Fakta',8), ('explainer','Explainer',9),
    ('kabar-duka','Kabar Duka',10)) AS c(slug, name, pos) WHERE sites.key='voxin';
UPDATE categories SET in_nav=false WHERE slug='kabar-duka';

INSERT INTO pages (site_id, slug, title, position, body_md) SELECT id, p.slug, p.title, p.pos, p.body FROM sites, (VALUES
    ('redaksi','Susunan Redaksi',1,'## Penanggung jawab\n\n[Nama Pemimpin Redaksi], Pemimpin Redaksi (UKW Utama, nomor sertifikat [...])\n\n## Redaksi\n\n[Nama], Editor\n\n[Nama], Wartawan, wilayah tugas [...]\n\n## Perusahaan\n\n[Nama PT Pers]\n\n[Alamat redaksi]\n\nEmail: halo@voxin.id'),
    ('pedoman-media-siber','Pedoman Pemberitaan Media Siber',2,'[Tempel Pedoman Pemberitaan Media Siber dari Dewan Pers di sini.]'),
    ('kode-etik','Kode Etik Jurnalistik',3,'[Tempel Kode Etik Jurnalistik dari Dewan Pers di sini.]'),
    ('koreksi-hak-jawab','Kebijakan Koreksi & Hak Jawab',4,'Kesalahan dalam berita kami perbaiki secara terbuka dengan catatan koreksi berwaktu di bawah artikel.\n\nUntuk menyampaikan hak jawab, kirim email ke halo@voxin.id dengan menyertakan tautan artikel dan bagian yang ingin ditanggapi.'),
    ('kerja-sama','Kebijakan Iklan & Kerja Sama',5,'Konten kerja sama selalu diberi label **KERJA SAMA**, tetap disunting redaksi, dan dipisahkan dari liputan independen.'),
    ('kontak','Kontak',6,'Email: halo@voxin.id\n\nAlamat: [Alamat redaksi]'),
    ('privasi','Kebijakan Privasi',7,'voxin.id tidak memakai cookie pelacak pihak ketiga. Statistik pembaca dihitung tanpa menyimpan alamat IP.'),
    ('tentang','Tentang VOXIN',0,'VOXIN Media menyajikan berita penting dan viral, langsung ke intinya. *Suarain yang penting.*\n\nKonten yang disusun dengan bantuan AI selalu diperiksa editor manusia sebelum terbit.')
    ) AS p(slug, title, pos, body) WHERE sites.key='voxin';
UPDATE pages SET body_md = replace(body_md, '\n', E'\n');
