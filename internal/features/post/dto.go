package post

import "time"

// Categories adalah daftar kategori artikel yang diizinkan.
var Categories = []string{"Teknologi", "Pendidikan", "Gaya Hidup", "Bisnis", "Opini", "Umum"}

// IsValidCategory memeriksa apakah kategori termasuk daftar yang diizinkan.
func IsValidCategory(category string) bool {
	for _, c := range Categories {
		if c == category {
			return true
		}
	}
	return false
}

// PostRequest adalah payload membuat/memperbarui artikel.
// IsPublished bertipe pointer: nil berarti status publikasi tidak diubah.
type PostRequest struct {
	Title          string `json:"title" binding:"required,max=255"`
	Category       string `json:"category"`
	Excerpt        string `json:"excerpt"`
	Content        string `json:"content" binding:"required"`
	CoverImagePath string `json:"cover_image_path"`
	IsPublished    *bool  `json:"is_published"`
}

// UpdatePostRequest adalah payload pembaruan artikel (PUT/PATCH).
// Seluruh field opsional: nil berarti field tidak diubah.
type UpdatePostRequest struct {
	Title          *string `json:"title" binding:"omitempty,max=255"`
	Category       *string `json:"category"`
	Excerpt        *string `json:"excerpt"`
	Content        *string `json:"content"`
	CoverImagePath *string `json:"cover_image_path"`
	IsPublished    *bool   `json:"is_published"`
}

// AuthorInfo adalah data ringkas penulis artikel.
type AuthorInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

// PostResponse adalah representasi artikel pada API.
type PostResponse struct {
	ID             uint        `json:"id"`
	UserID         uint        `json:"user_id"`
	Title          string      `json:"title"`
	Slug           string      `json:"slug"`
	Category       string      `json:"category"`
	Excerpt        string      `json:"excerpt"`
	Content        string      `json:"content"`
	CoverImagePath string      `json:"cover_image_path"`
	IsPublished    bool        `json:"is_published"`
	PublishedAt    *time.Time  `json:"published_at"`
	ReadingTime    int         `json:"reading_time"`
	Author         *AuthorInfo `json:"author,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// PublicPostResponse adalah representasi artikel untuk endpoint publik
// (tanpa data internal seperti user_id).
type PublicPostResponse struct {
	ID             uint        `json:"id"`
	Title          string      `json:"title"`
	Slug           string      `json:"slug"`
	Category       string      `json:"category"`
	Excerpt        string      `json:"excerpt"`
	Content        string      `json:"content"`
	CoverImagePath string      `json:"cover_image_path"`
	PublishedAt    *time.Time  `json:"published_at"`
	ReadingTime    int         `json:"reading_time"`
	AuthorName     string      `json:"author_name"`
	Author         *AuthorInfo `json:"author,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Stats adalah ringkasan 3 kartu statistik penulis.
// Field total/total_terbit dipertahankan sebagai alias bagi frontend lama.
type Stats struct {
	TotalArticles int64 `json:"total_articles"`
	TotalSelesai  int64 `json:"total_selesai"`
	BelumSelesai  int64 `json:"belum_selesai"`
	Total         int64 `json:"total"`
	TotalTerbit   int64 `json:"total_terbit"`
}

// UserSummary adalah ringkasan pengguna untuk monitoring admin.
type UserSummary struct {
	ID             uint   `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	TotalPosts     int64  `json:"total_posts"`
	TotalArticles  int64  `json:"total_articles"`
	TotalPublished int64  `json:"total_published"`
}

// ListFilter adalah filter pencarian artikel publik.
type ListFilter struct {
	Query    string
	Category string
}
