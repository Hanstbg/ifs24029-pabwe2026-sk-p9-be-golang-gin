# Delcom Blog API — Golang Gin (PABWE 2026 P9, Studi Kasus)

Backend REST API blog multi-tier dengan **Clean Architecture & Feature-First**:
Golang (Gin) · GORM · PostgreSQL 17 · JWT · Swagger UI · AI Blog Generator
yang terintegrasi dengan Search Engine + OpenAI Chat Completions.

## Menjalankan

```bash
psql -U postgres -c "CREATE DATABASE pabwe_p9_blog;"
cp .env.example .env          # sesuaikan DB_* dan OPENAI_* bila perlu
go mod tidy                   # mengunduh dependensi & membuat go.sum
go run cmd/server/main.go     # http://localhost:8081
```

- Swagger UI: http://localhost:8081/swagger/index.html (spesifikasi dinamis di `/api.json`)
- Akun bawaan (seeder): `admin / admin` (role `admin`) dan `member / member` (role `member`)

## Pengujian (TDD, target 100% statement coverage)

```bash
go test -v -cover ./internal/...
```

Pengujian memakai SQLite in-memory sehingga tidak membutuhkan PostgreSQL maupun
koneksi internet (Search Engine & OpenAI ditiru dengan `httptest`).

## Build

```bash
go build -o server.exe cmd/server/main.go   # Windows
go build -o server cmd/server/main.go       # Linux/macOS
```

`.env` harus berada di direktori yang sama saat biner dijalankan.

## Endpoint

| Method | Endpoint | Akses |
| --- | --- | --- |
| GET | `/swagger/*any`, `/api.json` | Publik |
| GET | `/api/public/categories` | Publik |
| GET | `/api/public/posts?q=&category=` | Publik |
| GET | `/api/public/posts/:slug` | Publik |
| POST | `/api/auth/register`, `/api/auth/login` | Publik |
| POST | `/api/auth/logout` | Login |
| GET/PUT | `/api/auth/profile` | Login |
| GET/POST | `/api/posts` | Penulis |
| GET | `/api/posts/stats` | Penulis |
| POST | `/api/posts/ai-generate` | Penulis |
| GET/PUT/DELETE | `/api/posts/:id` | Penulis (milik sendiri) |
| PATCH | `/api/posts/:id/toggle-publish` | Penulis (milik sendiri) |
| GET | `/api/admin/users`, `/api/admin/posts`, `/api/admin/users/:id/posts` | Admin |

Format respons seragam: `{"status": "success|fail|error", "message": "...", "data": ...}`.

## Catatan desain

- Slug unik: judul di-slugify, bentrok diberi akhiran `-2`, `-3`, ...
- Reading time = jumlah kata / 200 wpm (minimal 1 menit).
- `is_published` pada `POST/PUT /api/posts` bertipe opsional; bila dikosongkan pada `PUT`,
  status publikasi tidak berubah.
- Artikel milik penulis lain dijawab `403`; artikel yang tidak ada dijawab `404`.
- `POST /api/auth/register` mengembalikan `{token, user}`; login menerima username **atau** email; username/email duplikat dijawab `400`.
- `PUT/PATCH /api/posts/:id` bersifat parsial (field yang tidak dikirim tidak diubah).
- `GET /api/posts/stats` memakai `total_articles`, `total_selesai`, `belum_selesai` (alias lama `total`, `total_terbit` tetap ada).
- Endpoint `/api/public/*` tidak menampilkan `user_id`.
- **Asumsi kontrak Search Engine** (`SEARCH_ENGINE_ENDPOINT`): `GET ?q=<topik>` dengan header
  `Authorization: Bearer <OPENAI_KEY>`, membalas `{"results":[{"title","url","snippet"}]}`.
  Bila API Delcom berbeda, cukup sesuaikan `internal/features/ai/search.go`.
- `.gitignore` memakai awalan `/` pada biner (`/server`, `/main`, `/app`) agar folder
  `internal/server/` tidak ikut terabaikan oleh pipeline CI/CD (Jenkins).
