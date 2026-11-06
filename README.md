# voxin-portal

Portal berita **voxin.id** dan CMS redaksinya. Satu program Go, dua alamat:

| Alamat | Untuk | Port bawaan |
| --- | --- | --- |
| voxin.id | Pembaca: beranda, artikel, kategori, rubrik daerah, penulis, cari, halaman wajib | 8090 |
| cms.voxin.id | Redaksi: tulis & sunting artikel, terbitkan, daerah, tim, halaman wajib, analitik | 8091 |

Stack: Go (stdlib `net/http` + `html/template`), HTMX di CMS, PostgreSQL. Satu-satunya dependensi Go adalah `lib/pq`. Tanpa build frontend.

Tersambung ke [voxin-engine](https://github.com/hilmimuharromi/voxin-engine) lewat webhook: tombol "Terbitkan di voxin.id" di engine mengirim artikel beserta **foto aslinya** (base64 di `image_data`, kredit di `image_credit`) ke antrean editor portal. Foto diproses pipeline media CMS seperti unggahan biasa; `image_url` tetap diterima sebagai cadangan. Kirim ulang artikel yang sama hanya menambahkan foto bila artikelnya belum punya foto.

## Menjalankan di Mac

```bash
brew install go postgresql@16
brew services start postgresql@16
createdb voxin_portal

cp .env.example .env    # isi PORTAL_SECRET_KEY, email & password akun pertama
go run ./cmd/portal     # atau: make run
```

- Situs: http://localhost:8090
- CMS: http://localhost:8091 (masuk dengan `PORTAL_ADMIN_EMAIL` / `PORTAL_ADMIN_PASSWORD`, lalu ganti password di profil)

Database dan semua tabel dibuat otomatis saat program jalan.

### Dengan Docker

```bash
cp .env.example .env
docker compose up -d --build
```

## Langkah pertama di CMS

1. **Pengaturan**: isi nama PT, alamat redaksi, status verifikasi Dewan Pers, akun medsos. Semuanya tampil di footer.
2. **Halaman wajib**: lengkapi Susunan Redaksi, Pedoman Pemberitaan Media Siber, Kode Etik, dan lainnya. Halaman yang masih berisi `[placeholder]` ditandai.
3. **Tim**: tambahkan wartawan/editor beserta jabatan, wilayah tugas, dan jenjang UKW. Profil ini tampil di halaman penulis.
4. **Daerah**: tambahkan kabupaten/kota. Tandai "Mitra" untuk daerah dengan kerja sama aktif dan upload logonya.
5. **Sambungkan engine**: Pengaturan › Sambungan VOXIN Engine › "Buat token". Salin webhook dan token ke Pengaturan engine (Webhook portal + Token portal).

## Alur redaksi

| Peran | Bisa |
| --- | --- |
| Kontributor, Wartawan | Menulis artikel sendiri, upload foto, kirim ke editor |
| Editor | Semua di atas + sunting artikel siapa pun, terbitkan, jadwalkan, turunkan, analitik |
| Pemimpin Redaksi, Admin | Semua di atas + tim, daerah, halaman wajib, pengaturan |

Status artikel: **Draft → Perlu ditinjau → Terjadwal/Terbit → Diturunkan**.

Jenis artikel dan aturannya (dikunci sistem):

| Jenis | Tampil di situs | Syarat terbit |
| --- | --- | --- |
| Berita | Byline wartawan | Penulis, kategori, isi ≥ 40 kata, kredit foto |
| Kurasi | "Dirangkum dari [media]" | + nama media sumber |
| Kerja Sama | Label **KERJA SAMA** + keterangan mitra | + daerah |
| Opini | Label **OPINI** | Penulis |

- Artikel dari engine masuk sebagai **Kurasi** berstatus *Perlu ditinjau*, tidak pernah langsung terbit. Kalau terbit tanpa penulis, editor yang menerbitkan tercatat sebagai penulis.
- Ganti slug artikel yang sudah terbit otomatis membuat redirect 301 dari link lama.
- Artikel yang diturunkan tidak dihapus: halamannya menampilkan alasan (HTTP 410), sesuai pedoman media siber.
- Setiap perubahan judul/isi tersimpan di riwayat versi.

## SEO

- Schema.org `NewsArticle` di setiap artikel, `NewsMediaOrganization` di beranda.
- `/sitemap.xml`, `/sitemap-news.xml` (artikel 48 jam terakhir, untuk Google News), `/rss.xml`, `/robots.txt`.
- Open Graph + Twitter Card dengan foto 1200 px.
- Foto otomatis dibuat versi 400/800/1200 px untuk pembaca HP.
- Halaman publik di-cache 60 detik; cache dibersihkan setiap ada perubahan di CMS.

## Analitik

Dihitung sendiri tanpa cookie pelacak: skrip kecil di halaman artikel mengirim satu tayangan per buka, bot tidak dihitung, pembaca unik dihitung per hari dari hash yang tidak bisa dibalik (IP tidak disimpan). Sumber pembaca dikelompokkan: mesin pencari, media sosial, langsung, situs lain.

## Struktur

```
cmd/portal/        main: config, DB, dua server (publik & CMS), penjadwal terbit
internal/config/   env
internal/store/    skema PostgreSQL (migrasi otomatis) + query
internal/core/     aturan terbit, cache halaman, penjadwal
internal/site/     situs publik: template, CSS, SEO, hitung tayangan
internal/cms/      CMS: login, editor, daerah, tim, analitik, pengaturan, API engine
internal/media/    simpan foto + varian ukuran
internal/auth/     password (PBKDF2) + sesi
internal/text/     Markdown aman, slug, tanggal Indonesia
```

Semua tabel konten punya kolom `site_id`, jadi situs kedua (mis. terasberita.id) bisa ditambahkan ke CMS yang sama nanti tanpa mengubah skema.

## Belum ada (fase berikutnya, sesuai PRD)

- Kontrak kerja sama pemda + kuota, dan laporan kerja sama PDF/XLSX.
- Halaman `/koreksi` dan formulir hak jawab publik, inbox rilis humas.
- Editor rich text (sekarang Markdown + pratinjau langsung).
- 2FA untuk pemred/admin.
